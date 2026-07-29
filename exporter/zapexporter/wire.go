package zapexporter

import (
	"encoding/json"
	"fmt"

	luxzap "github.com/luxfi/zap"
)

// The OTLZ wire: a JSON batch inside a luxfi/zap envelope, tagged by message
// type. This is what o11y's zapreceiver / zaplogreceiver / zapmetricreceiver
// decode, and what luxfi/trace emits.
//
// WHY THIS FILE EXISTS. This exporter previously marshalled OTLP protobuf and
// POSTed it to "/v1/logs" over zap-proto/http. That is a different protocol
// entirely, so the agent and the collector could not talk despite sharing a
// port: the collector's zap.Node never saw an envelope whose type it recognised,
// so its handler was never invoked (not one log line, not even the Warn drop
// paths), nothing was written back, and the agent sat in `read response` until
// it timed out. Measured on 2026-07-29: o11y_logs.logs_v2 took 0 rows for two
// days while the agent retried on backoff. Bumping zap-proto/http and
// zap-proto/go was chasing the wrong library — the receiver does not speak that
// protocol at any version.
//
// The batch structs are LOCAL COPIES of the receiver's, deliberately. Importing
// hanzoai/o11y here would pull a whole collector distribution into a node agent
// to obtain four struct definitions. The receiver does the same thing for the
// same reason ("Field layout mirrors luxfi/metric.MetricBatch — keep in
// lockstep"), so the convention is established; the JSON tags are the contract
// and wire_test.go pins them.

// Message types, from o11y/pkg/zap*receiver. These are the ZAP MsgType values
// the collector dispatches on, and they are wire constants — a mismatch here is
// silent, because an unrecognised type is simply never handed to a handler.
const (
	msgSpanBatch   uint16 = 1
	msgMetricBatch uint16 = 2
	msgLogBatch    uint16 = 3
)

// spanBatch mirrors o11y/pkg/zapreceiver.SpanBatch.
type spanBatch struct {
	AppName  string            `json:"appName,omitempty"`
	Version  string            `json:"version,omitempty"`
	Resource map[string]string `json:"resource,omitempty"`
	Spans    []span            `json:"spans"`
}

type span struct {
	TraceID      string         `json:"traceId"`
	SpanID       string         `json:"spanId"`
	ParentSpanID string         `json:"parentSpanId,omitempty"`
	Name         string         `json:"name"`
	Kind         string         `json:"kind,omitempty"`
	StartUnixNs  int64          `json:"startUnixNs"`
	EndUnixNs    int64          `json:"endUnixNs"`
	Attributes   map[string]any `json:"attributes,omitempty"`
	Events       []spanEvent    `json:"events,omitempty"`
	StatusCode   string         `json:"statusCode,omitempty"`
	StatusMsg    string         `json:"statusMessage,omitempty"`
}

type spanEvent struct {
	Name       string         `json:"name"`
	TimeUnixNs int64          `json:"timeUnixNs"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// logBatch mirrors o11y/pkg/zaplogreceiver.LogBatch.
type logBatch struct {
	AppName  string            `json:"appName,omitempty"`
	Version  string            `json:"version,omitempty"`
	Resource map[string]string `json:"resource,omitempty"`
	Records  []logRecord       `json:"records"`
}

type logRecord struct {
	TimeUnixNs         int64          `json:"timeUnixNs"`
	ObservedTimeUnixNs int64          `json:"observedTimeUnixNs,omitempty"`
	Severity           int            `json:"severity,omitempty"`
	SeverityText       string         `json:"severityText,omitempty"`
	Body               string         `json:"body,omitempty"`
	Attributes         map[string]any `json:"attributes,omitempty"`
	TraceID            string         `json:"traceId,omitempty"`
	SpanID             string         `json:"spanId,omitempty"`
	EventName          string         `json:"eventName,omitempty"`
}

// metricBatch mirrors o11y/pkg/zapmetricreceiver.MetricBatch, whose layout in
// turn mirrors luxfi/metric — a Prometheus-shaped model, not OTLP's.
type metricBatch struct {
	AppName     string            `json:"appName,omitempty"`
	Version     string            `json:"version,omitempty"`
	Resource    map[string]string `json:"resource,omitempty"`
	TimestampNs int64             `json:"timestampNs"`
	Families    []metricFamily    `json:"families"`
}

type metricFamily struct {
	Name    string   `json:"name"`
	Help    string   `json:"help,omitempty"`
	Type    string   `json:"type"` // counter | gauge | histogram | summary
	Metrics []metric `json:"metrics"`
}

type metric struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Value       *float64          `json:"value,omitempty"`
	SampleCount *uint64           `json:"sampleCount,omitempty"`
	SampleSum   *float64          `json:"sampleSum,omitempty"`
	Buckets     []bucket          `json:"buckets,omitempty"`
	Quantiles   []quantile        `json:"quantiles,omitempty"`
}

type bucket struct {
	UpperBound      float64 `json:"upperBound"`
	CumulativeCount uint64  `json:"cumulativeCount"`
}

type quantile struct {
	Quantile float64 `json:"quantile"`
	Value    float64 `json:"value"`
}

// encode marshals a batch to JSON and wraps it in a ZAP envelope tagged with
// msgType.
//
// The type goes in the UPPER 8 BITS of the flags field — FinishWithFlags(t<<8) —
// which is the one detail that cannot be inferred from the receiver side and the
// one that makes the difference between a dispatched batch and a silent drop.
func encode(msgType uint16, batch any) (*luxzap.Message, error) {
	payload, err := json.Marshal(batch)
	if err != nil {
		return nil, fmt.Errorf("zapexporter: marshal batch: %w", err)
	}
	const envelopeSize = 16
	b := luxzap.NewBuilder(envelopeSize + 64 + len(payload))
	root := b.StartObject(envelopeSize)
	root.SetBytes(0, payload)
	root.FinishAsRoot()
	msg, err := luxzap.Parse(b.FinishWithFlags(msgType << 8))
	if err != nil {
		return nil, fmt.Errorf("zapexporter: parse outgoing envelope: %w", err)
	}
	return msg, nil
}
