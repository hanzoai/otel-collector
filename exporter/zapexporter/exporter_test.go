package zapexporter

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	zaphttp "github.com/zap-proto/http"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
)

// TestExportTracesOverZAP proves the exporter ships OTLP over the ZAP wire:
// a real zaphttp.Server terminates the frame, and the body decodes back to the
// spans we sent.
func TestExportTracesOverZAP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var (
		mu       sync.Mutex
		gotPath  string
		gotSpans int
	)
	srv := &zaphttp.Server{Handler: func(ctx *fasthttp.RequestCtx) {
		req := ptraceotlp.NewExportRequest()
		if err := req.UnmarshalProto(ctx.PostBody()); err != nil {
			ctx.SetStatusCode(400)
			return
		}
		mu.Lock()
		gotPath = string(ctx.Path())
		gotSpans = req.Traces().SpanCount()
		mu.Unlock()
		ctx.SetStatusCode(200)
	}}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	cfg := NewFactory().CreateDefaultConfig().(*Config)
	cfg.Endpoint = ln.Addr().String()

	exp, err := NewFactory().CreateTraces(context.Background(), exportertest.NewNopSettings(typeStr), cfg)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := exp.Start(context.Background(), componenttest.NewNopHost()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = exp.Shutdown(context.Background()) }()

	td := ptrace.NewTraces()
	td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetName("s")

	if err := exp.ConsumeTraces(context.Background(), td); err != nil {
		t.Fatalf("consume: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		p, n := gotPath, gotSpans
		mu.Unlock()
		if p == "/v1/traces" && n == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server did not receive the span over ZAP (path=%q spans=%d)", gotPath, gotSpans)
}
