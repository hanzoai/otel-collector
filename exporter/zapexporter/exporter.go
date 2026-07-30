package zapexporter

import (
	"context"
	"fmt"
	"strings"
	"sync"

	luxzap "github.com/luxfi/zap"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// zapExporter ships pdata to an o11y ZAP receiver as a JSON batch inside a
// luxfi/zap envelope. See wire.go for why the previous OTLP-protobuf-over-
// zap-proto/http approach could not work and what it cost.
//
// The receiver answers NOTHING — its handler returns a nil message on every path
// and its own doc says "the sender doesn't wait on the response". So this uses
// Send, not Call. Waiting for a reply is what produced two days of `i/o timeout`
// against a receiver behaving exactly as designed.
type zapExporter struct {
	cfg      *Config
	settings exporter.Settings

	mu     sync.Mutex
	node   *luxzap.Node
	peerID string
}

func newExporter(set exporter.Settings, cfg *Config) *zapExporter {
	return &zapExporter{cfg: cfg, settings: set}
}

// Start brings up the local ZAP node. A failed dial is deliberately NOT fatal:
// this runs as a DaemonSet on every node and the collector may be unreachable at
// boot, so it must not take the agent down — send() reconnects.
func (e *zapExporter) Start(context.Context, component.Host) error {
	nodeID := "otel-agent"
	if id := e.settings.ID.String(); id != "" {
		nodeID = "otel-agent-" + strings.ReplaceAll(id, "/", "-")
	}
	cfg := luxzap.NodeConfig{
		NodeID:      nodeID,
		ServiceType: "_o11y._tcp",
		Port:        0,
		NoDiscovery: true,
	}
	// A UDS endpoint needs no transport selection — luxfi/zap's Network() reads
	// the address shape, so a socket path dials unix on both ends. QUIC is the
	// remote case, and its TLS 1.3 negotiates X25519MLKEM768 (X-Wing) by default
	// on Go 1.26, so a cross-machine hop is quantum-secure without this exporter
	// configuring any crypto.
	if e.cfg.Transport == "quic" {
		cfg.Transport = luxzap.TransportQUIC
	}
	e.node = luxzap.NewNode(cfg)
	if err := e.node.Start(); err != nil {
		return fmt.Errorf("zapexporter: zap node start (transport=%s): %w", e.transportName(), err)
	}
	if err := e.connect(); err != nil {
		e.settings.Logger.Debug("OTLZ exporter: initial connect failed; will retry on export",
			zap.String("endpoint", e.cfg.Endpoint), zap.Error(err))
	}
	e.settings.Logger.Info("OTLZ exporter ready (wire=luxfi/zap envelope, JSON batch)",
		zap.String("endpoint", e.cfg.Endpoint),
		zap.String("transport", e.transportName()))
	return nil
}

// transportName reports the transport actually in use, for the one log line an
// operator reads to confirm a hop is not silently plaintext across machines.
func (e *zapExporter) transportName() string {
	if isSocketPath(e.cfg.Endpoint) {
		return "uds"
	}
	if e.cfg.Transport == "quic" {
		return "quic+x25519mlkem768"
	}
	return "tcp"
}

func (e *zapExporter) Shutdown(context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.node != nil {
		e.node.Stop()
	}
	return nil
}

func (e *zapExporter) connect() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.connectLocked()
}

func (e *zapExporter) connectLocked() error {
	if e.peerID != "" {
		return nil
	}
	if err := e.node.ConnectDirect(e.cfg.Endpoint); err != nil {
		return err
	}
	peers := e.node.Peers()
	if len(peers) == 0 {
		return fmt.Errorf("zapexporter: connected to %s but no peer ID", e.cfg.Endpoint)
	}
	e.peerID = peers[0]
	return nil
}

// send delivers one envelope, redialing if the cached peer is stale.
//
// A failure is returned as RETRYABLE (exporterhelper backs off and retries with
// the batch intact) rather than swallowed. Dropping a node's logs quietly is how
// a fleet loses two days of telemetry without one error surfacing.
func (e *zapExporter) send(ctx context.Context, msg *luxzap.Message) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.connectLocked(); err != nil {
		return fmt.Errorf("zapexporter: connect %s: %w", e.cfg.Endpoint, err)
	}
	if err := e.node.Send(ctx, e.peerID, msg); err != nil {
		e.peerID = "" // stale connection; next call redials
		return fmt.Errorf("zapexporter: send to %s: %w", e.cfg.Endpoint, err)
	}
	return nil
}

func (e *zapExporter) pushTraces(ctx context.Context, td ptrace.Traces) error {
	for _, batch := range tracesToBatches(td) {
		msg, err := encode(msgSpanBatch, batch)
		if err != nil {
			return consumererror.NewPermanent(err)
		}
		if err := e.send(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

func (e *zapExporter) pushLogs(ctx context.Context, ld plog.Logs) error {
	for _, batch := range logsToBatches(ld) {
		msg, err := encode(msgLogBatch, batch)
		if err != nil {
			return consumererror.NewPermanent(err)
		}
		if err := e.send(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

func (e *zapExporter) pushMetrics(ctx context.Context, md pmetric.Metrics) error {
	batches, skipped := metricsToBatches(md)
	if skipped > 0 {
		// Named rather than silently dropped: the wire is Prometheus-shaped
		// (counter/gauge/histogram/summary), so an OTLP type with no counterpart
		// has nowhere to go. Approximating one would corrupt the series.
		e.settings.Logger.Warn("OTLZ exporter: metric types with no wire counterpart were skipped",
			zap.Int("skipped", skipped))
	}
	for _, batch := range batches {
		msg, err := encode(msgMetricBatch, batch)
		if err != nil {
			return consumererror.NewPermanent(err)
		}
		if err := e.send(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

// ── pdata -> wire ───────────────────────────────────────────────────────────
//
// One ResourceSpans/ResourceLogs becomes one batch, because a batch carries
// exactly one service's identity (appName + resource). Instrumentation scope is
// dropped: the wire has no field for it, and inventing one would put a name on
// the wire that the emitter never sent.

// resourceOf splits a resource into the batch's service identity and the rest of
// its attributes. service.name and service.version are lifted out because the
// wire carries them as their own fields.
func resourceOf(attrs pcommon.Map) (appName, version string, rest map[string]string) {
	rest = map[string]string{}
	attrs.Range(func(k string, v pcommon.Value) bool {
		switch k {
		case "service.name":
			appName = v.AsString()
		case "service.version":
			version = v.AsString()
		default:
			rest[k] = v.AsString()
		}
		return true
	})
	if len(rest) == 0 {
		rest = nil
	}
	return appName, version, rest
}

func attrsOf(attrs pcommon.Map) map[string]any {
	if attrs.Len() == 0 {
		return nil
	}
	out := make(map[string]any, attrs.Len())
	attrs.Range(func(k string, v pcommon.Value) bool {
		out[k] = v.AsRaw()
		return true
	})
	return out
}

func tracesToBatches(td ptrace.Traces) []*spanBatch {
	var out []*spanBatch
	rss := td.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		app, ver, res := resourceOf(rs.Resource().Attributes())
		b := &spanBatch{AppName: app, Version: ver, Resource: res, Spans: []span{}}
		sss := rs.ScopeSpans()
		for j := 0; j < sss.Len(); j++ {
			spans := sss.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				b.Spans = append(b.Spans, translateSpan(spans.At(k)))
			}
		}
		if len(b.Spans) > 0 {
			out = append(out, b)
		}
	}
	return out
}

func translateSpan(s ptrace.Span) span {
	tid := s.TraceID()
	sid := s.SpanID()
	dst := span{
		TraceID:     fmt.Sprintf("%x", tid[:]),
		SpanID:      fmt.Sprintf("%x", sid[:]),
		Name:        s.Name(),
		Kind:        s.Kind().String(),
		StartUnixNs: int64(s.StartTimestamp()),
		EndUnixNs:   int64(s.EndTimestamp()),
		Attributes:  attrsOf(s.Attributes()),
		StatusMsg:   s.Status().Message(),
	}
	if psid := s.ParentSpanID(); !psid.IsEmpty() {
		dst.ParentSpanID = fmt.Sprintf("%x", psid[:])
	}
	if c := s.Status().Code(); c != ptrace.StatusCodeUnset {
		dst.StatusCode = c.String()
	}
	evs := s.Events()
	for i := 0; i < evs.Len(); i++ {
		ev := evs.At(i)
		dst.Events = append(dst.Events, spanEvent{
			Name:       ev.Name(),
			TimeUnixNs: int64(ev.Timestamp()),
			Attributes: attrsOf(ev.Attributes()),
		})
	}
	return dst
}

func logsToBatches(ld plog.Logs) []*logBatch {
	var out []*logBatch
	rls := ld.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		rl := rls.At(i)
		app, ver, res := resourceOf(rl.Resource().Attributes())
		b := &logBatch{AppName: app, Version: ver, Resource: res, Records: []logRecord{}}
		sls := rl.ScopeLogs()
		for j := 0; j < sls.Len(); j++ {
			recs := sls.At(j).LogRecords()
			for k := 0; k < recs.Len(); k++ {
				b.Records = append(b.Records, translateLog(recs.At(k)))
			}
		}
		if len(b.Records) > 0 {
			out = append(out, b)
		}
	}
	return out
}

func translateLog(r plog.LogRecord) logRecord {
	dst := logRecord{
		TimeUnixNs:         int64(r.Timestamp()),
		ObservedTimeUnixNs: int64(r.ObservedTimestamp()),
		Severity:           int(r.SeverityNumber()),
		SeverityText:       r.SeverityText(),
		Body:               r.Body().AsString(),
		Attributes:         attrsOf(r.Attributes()),
		EventName:          r.EventName(),
	}
	if tid := r.TraceID(); !tid.IsEmpty() {
		dst.TraceID = fmt.Sprintf("%x", tid[:])
	}
	if sid := r.SpanID(); !sid.IsEmpty() {
		dst.SpanID = fmt.Sprintf("%x", sid[:])
	}
	return dst
}

// metricsToBatches converts what maps and COUNTS what does not.
//
// The wire is Prometheus-shaped, so Gauge and Sum become gauge/counter and
// Histogram becomes histogram with cumulative buckets. ExponentialHistogram and
// Summary have no faithful representation here and are reported as skipped
// rather than approximated.
func metricsToBatches(md pmetric.Metrics) (out []*metricBatch, skipped int) {
	rms := md.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		rm := rms.At(i)
		app, ver, res := resourceOf(rm.Resource().Attributes())
		b := &metricBatch{AppName: app, Version: ver, Resource: res, Families: []metricFamily{}}
		sms := rm.ScopeMetrics()
		for j := 0; j < sms.Len(); j++ {
			ms := sms.At(j).Metrics()
			for k := 0; k < ms.Len(); k++ {
				fam, ts, ok := translateMetric(ms.At(k))
				if !ok {
					skipped++
					continue
				}
				if ts > b.TimestampNs {
					b.TimestampNs = ts
				}
				b.Families = append(b.Families, fam)
			}
		}
		if len(b.Families) > 0 {
			out = append(out, b)
		}
	}
	return out, skipped
}

func translateMetric(m pmetric.Metric) (fam metricFamily, latestNs int64, ok bool) {
	fam = metricFamily{Name: m.Name(), Help: m.Description()}
	switch m.Type() {
	case pmetric.MetricTypeGauge:
		fam.Type = "gauge"
		dps := m.Gauge().DataPoints()
		for i := 0; i < dps.Len(); i++ {
			dp := dps.At(i)
			v := numberValue(dp)
			fam.Metrics = append(fam.Metrics, metric{Labels: labelsOf(dp.Attributes()), Value: &v})
			latestNs = maxNs(latestNs, int64(dp.Timestamp()))
		}
	case pmetric.MetricTypeSum:
		fam.Type = "counter"
		dps := m.Sum().DataPoints()
		for i := 0; i < dps.Len(); i++ {
			dp := dps.At(i)
			v := numberValue(dp)
			fam.Metrics = append(fam.Metrics, metric{Labels: labelsOf(dp.Attributes()), Value: &v})
			latestNs = maxNs(latestNs, int64(dp.Timestamp()))
		}
	case pmetric.MetricTypeHistogram:
		fam.Type = "histogram"
		dps := m.Histogram().DataPoints()
		for i := 0; i < dps.Len(); i++ {
			dp := dps.At(i)
			count := dp.Count()
			sum := dp.Sum()
			mm := metric{Labels: labelsOf(dp.Attributes()), SampleCount: &count, SampleSum: &sum}
			// OTLP bucket counts are per-bucket; this wire wants cumulative.
			var cum uint64
			bounds := dp.ExplicitBounds()
			counts := dp.BucketCounts()
			for bi := 0; bi < bounds.Len() && bi < counts.Len(); bi++ {
				cum += counts.At(bi)
				mm.Buckets = append(mm.Buckets, bucket{UpperBound: bounds.At(bi), CumulativeCount: cum})
			}
			fam.Metrics = append(fam.Metrics, mm)
			latestNs = maxNs(latestNs, int64(dp.Timestamp()))
		}
	default:
		return metricFamily{}, 0, false
	}
	if len(fam.Metrics) == 0 {
		return metricFamily{}, 0, false
	}
	return fam, latestNs, true
}

func numberValue(dp pmetric.NumberDataPoint) float64 {
	if dp.ValueType() == pmetric.NumberDataPointValueTypeInt {
		return float64(dp.IntValue())
	}
	return dp.DoubleValue()
}

func labelsOf(attrs pcommon.Map) map[string]string {
	if attrs.Len() == 0 {
		return nil
	}
	out := make(map[string]string, attrs.Len())
	attrs.Range(func(k string, v pcommon.Value) bool {
		out[k] = v.AsString()
		return true
	})
	return out
}

func maxNs(a, b int64) int64 {
	if b > a {
		return b
	}
	return a
}
