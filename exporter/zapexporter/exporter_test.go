package zapexporter

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"strconv"
	"sync"
	"testing"
	"time"

	luxzap "github.com/luxfi/zap"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// The receiver these tests stand up is a real luxfi/zap node dispatching on
// MsgType, which is what o11y's zapreceiver/zaplogreceiver are.
//
// The test this replaces stood up a zaphttp.Server and asserted the exporter
// POSTed OTLP protobuf to "/v1/traces". It passed for two days while nothing
// reached production, because it validated the agent against the WRONG SERVER —
// the collector never spoke that protocol. A green test against a server nobody
// runs is worse than no test: it is a standing claim that the wire works.
//
// So these assert on the decoded batch a real zap node hands its handler. Run
// them against the old exporter and they fail with a frame-size error, which is
// the two protocols disagreeing.

type capture struct {
	mu      sync.Mutex
	logs    []logBatch
	spans   []spanBatch
	metrics []metricBatch
}

func (c *capture) add(msgType uint16, payload []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch msgType {
	case msgLogBatch:
		var b logBatch
		if json.Unmarshal(payload, &b) == nil {
			c.logs = append(c.logs, b)
		}
	case msgSpanBatch:
		var b spanBatch
		if json.Unmarshal(payload, &b) == nil {
			c.spans = append(c.spans, b)
		}
	case msgMetricBatch:
		var b metricBatch
		if json.Unmarshal(payload, &b) == nil {
			c.metrics = append(c.metrics, b)
		}
	}
}

func (c *capture) counts() (int, int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.logs), len(c.spans), len(c.metrics)
}

// receiver starts a zap node that records the batches it is handed, mirroring
// the o11y receivers: a handler per MsgType, payload at root bytes offset 0, and
// a nil reply — the sender does not wait on a response.
func receiver(t *testing.T) (addr string, got *capture) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close() // the node binds the port itself

	got = &capture{}
	node := luxzap.NewNode(luxzap.NodeConfig{
		NodeID:      "test-o11y-receiver",
		ServiceType: "_o11y._tcp",
		Port:        port,
		NoDiscovery: true,
	})
	for _, mt := range []uint16{msgLogBatch, msgSpanBatch, msgMetricBatch} {
		msgType := mt
		node.Handle(msgType, func(_ context.Context, _ string, msg *luxzap.Message) (*luxzap.Message, error) {
			got.add(msgType, msg.Root().Bytes(0))
			return nil, nil
		})
	}
	if err := node.Start(); err != nil {
		t.Fatalf("receiver start: %v", err)
	}
	t.Cleanup(node.Stop)
	return net.JoinHostPort("127.0.0.1", portStr), got
}

func eventually(t *testing.T, want func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if want() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(msg)
}

func TestExportLogsOverOTLZ(t *testing.T) {
	addr, got := receiver(t)

	cfg := NewFactory().CreateDefaultConfig().(*Config)
	cfg.Endpoint = addr
	exp, err := NewFactory().CreateLogs(context.Background(), exportertest.NewNopSettings(typeStr), cfg)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := exp.Start(context.Background(), componenttest.NewNopHost()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = exp.Shutdown(context.Background()) }()

	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "cart")
	rl.Resource().Attributes().PutStr("service.version", "1.2.3")
	rl.Resource().Attributes().PutStr("k8s.node.name", "node-a")
	rec := rl.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	rec.Body().SetStr("checkout failed")
	rec.SetSeverityNumber(plog.SeverityNumberError)
	rec.SetSeverityText("ERROR")

	if err := exp.ConsumeLogs(context.Background(), ld); err != nil {
		t.Fatalf("consume: %v", err)
	}
	eventually(t, func() bool { n, _, _ := got.counts(); return n == 1 },
		"receiver never got a log batch — the envelope did not dispatch")

	got.mu.Lock()
	b := got.logs[0]
	got.mu.Unlock()

	// service.name/version are lifted OUT of resource into their own fields.
	if b.AppName != "cart" || b.Version != "1.2.3" {
		t.Fatalf("identity not lifted: appName=%q version=%q", b.AppName, b.Version)
	}
	if _, dup := b.Resource["service.name"]; dup {
		t.Fatal("service.name left in resource as well as appName — duplicated identity")
	}
	if b.Resource["k8s.node.name"] != "node-a" {
		t.Fatalf("resource attribute lost: %v", b.Resource)
	}
	if len(b.Records) != 1 {
		t.Fatalf("want 1 record, got %d", len(b.Records))
	}
	r := b.Records[0]
	if r.Body != "checkout failed" || r.SeverityText != "ERROR" {
		t.Fatalf("record body/severity wrong: %+v", r)
	}
	// Severity text is carried verbatim rather than re-derived from the number,
	// so a service's own vocabulary survives.
	if r.Severity != int(plog.SeverityNumberError) {
		t.Fatalf("severity number wrong: %d", r.Severity)
	}
}

func TestExportTracesOverOTLZ(t *testing.T) {
	addr, got := receiver(t)

	cfg := NewFactory().CreateDefaultConfig().(*Config)
	cfg.Endpoint = addr
	exp, err := NewFactory().CreateTraces(context.Background(), exportertest.NewNopSettings(typeStr), cfg)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := exp.Start(context.Background(), componenttest.NewNopHost()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = exp.Shutdown(context.Background()) }()

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "checkout")
	s := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	s.SetName("POST /v1/checkout")
	s.SetTraceID(ptrace.NewTraces().ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty().TraceID())
	s.SetStartTimestamp(1000)
	s.SetEndTimestamp(2000)

	if err := exp.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("consume: %v", err)
	}
	eventually(t, func() bool { _, n, _ := got.counts(); return n == 1 },
		"receiver never got a span batch — the envelope did not dispatch")

	got.mu.Lock()
	b := got.spans[0]
	got.mu.Unlock()
	if b.AppName != "checkout" || len(b.Spans) != 1 {
		t.Fatalf("batch wrong: %+v", b)
	}
	if b.Spans[0].Name != "POST /v1/checkout" || b.Spans[0].StartUnixNs != 1000 || b.Spans[0].EndUnixNs != 2000 {
		t.Fatalf("span wrong: %+v", b.Spans[0])
	}
}

// TestEnvelopeCarriesMsgTypeInUpperFlagBits pins the one detail that cannot be
// inferred from the receiver and whose absence fails silently: the message type
// lives in the upper 8 bits of the ZAP flags field. Get it wrong and the node
// never dispatches, the handler never runs, and nothing is logged anywhere.
func TestEnvelopeCarriesMsgTypeInUpperFlagBits(t *testing.T) {
	for _, mt := range []uint16{msgSpanBatch, msgMetricBatch, msgLogBatch} {
		msg, err := encode(mt, &logBatch{Records: []logRecord{{Body: "x"}}})
		if err != nil {
			t.Fatalf("encode(%d): %v", mt, err)
		}
		if got := msg.Flags() >> 8; uint16(got) != mt {
			t.Fatalf("msgType %d not in upper flag bits: flags>>8 = %d", mt, got)
		}
		var back logBatch
		if err := json.Unmarshal(msg.Root().Bytes(0), &back); err != nil {
			t.Fatalf("payload not at root bytes 0 for msgType %d: %v", mt, err)
		}
		if len(back.Records) != 1 || back.Records[0].Body != "x" {
			t.Fatalf("payload round-trip wrong for msgType %d: %+v", mt, back)
		}
	}
}

// TestSendFailureIsRetryable proves a dead collector produces a retryable error
// rather than a silent drop. Swallowing it is how a fleet loses telemetry with no
// error surfaced anywhere.
func TestSendFailureIsRetryable(t *testing.T) {
	cfg := NewFactory().CreateDefaultConfig().(*Config)
	cfg.Endpoint = "127.0.0.1:1" // nothing listens

	e := newExporter(exportertest.NewNopSettings(typeStr), cfg)
	if err := e.Start(context.Background(), componenttest.NewNopHost()); err != nil {
		t.Fatalf("start must tolerate an unreachable collector: %v", err)
	}
	defer func() { _ = e.Shutdown(context.Background()) }()

	ld := plog.NewLogs()
	ld.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().Body().SetStr("x")

	err := e.pushLogs(context.Background(), ld)
	if err == nil {
		t.Fatal("a send to a dead collector returned nil — the batch would be dropped silently")
	}
}

// TestTransportLadder pins the locality rule: a socket path is UDS, a host:port
// is TCP, and "quic" is available rather than ErrTransportUnavailable.
//
// The QUIC arm is the load-bearing one. luxfi/zap only serves TransportQUIC when
// the quic subpackage has been linked in so its init registers the factory; miss
// that import and every remote hop silently falls back to an error at Start,
// which is how a cross-machine hop ends up plaintext without anyone noticing.
func TestTransportLadder(t *testing.T) {
	for _, tc := range []struct {
		endpoint, transport, want string
	}{
		{"/run/hanzo/o11y-logs.sock", "", "uds"},
		{"@o11y-logs", "", "uds"},
		{"cloud.hanzo.svc:4318", "", "tcp"},
		{"cloud.hanzo.svc:4318", "quic", "quic+x25519mlkem768"},
	} {
		cfg := NewFactory().CreateDefaultConfig().(*Config)
		cfg.Endpoint, cfg.Transport = tc.endpoint, tc.transport
		if err := cfg.Validate(); err != nil {
			t.Fatalf("validate %q/%q: %v", tc.endpoint, tc.transport, err)
		}
		e := newExporter(exportertest.NewNopSettings(typeStr), cfg)
		if got := e.transportName(); got != tc.want {
			t.Fatalf("endpoint %q transport %q: want %s, got %s", tc.endpoint, tc.transport, tc.want, got)
		}
	}
}

// TestQUICTransportIsLinkedAndNeedsTLS pins BOTH halves of the QUIC story, and
// the second half is the one that bites.
//
// The factory IS registered (the anonymous quic import in factory.go), so the
// failure is NOT ErrTransportUnavailable. But Start still refuses without
// NodeConfig.TLS carrying server certificates — QUIC has no plaintext mode. So
// selecting "quic" is necessary and not sufficient: the agent needs a TLS
// identity, which is a KMS-provisioned cert, not a config flag. This test states
// that plainly so nobody reads the config comment and assumes a remote hop is
// encrypted merely because transport says quic.
func TestQUICTransportIsLinkedAndNeedsTLS(t *testing.T) {
	n := luxzap.NewNode(luxzap.NodeConfig{
		NodeID:      "quic-linkage-probe",
		ServiceType: "_o11y._tcp",
		Port:        0,
		NoDiscovery: true,
		Transport:   luxzap.TransportQUIC,
	})
	err := n.Start()
	if err == nil {
		n.Stop()
		t.Fatal("QUIC started with no TLS material — luxfi/zap used to require a server cert; if that changed, drop the TLS deploy-gate from Config's docs")
	}
	if errors.Is(err, luxzap.ErrTransportUnavailable) {
		t.Fatalf("QUIC factory not registered — is _ \"github.com/luxfi/zap/quic\" still imported? %v", err)
	}
	if !strings.Contains(err.Error(), "TLS") {
		t.Fatalf("expected a TLS-material error, got: %v", err)
	}
}

// TestQUICOnASocketPathIsRefused: asking for QUIC on a UDS path is a config
// error, not a silent downgrade. Someone who wanted an encrypted remote hop and
// typed a socket path should hear it from validation.
func TestQUICOnASocketPathIsRefused(t *testing.T) {
	cfg := NewFactory().CreateDefaultConfig().(*Config)
	cfg.Endpoint, cfg.Transport = "/run/hanzo/o11y.sock", "quic"
	if err := cfg.Validate(); err == nil {
		t.Fatal("quic on a unix socket validated — the transport request was silently ignored")
	}
}
