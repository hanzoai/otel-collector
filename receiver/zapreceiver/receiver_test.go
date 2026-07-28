package zapreceiver

import (
	"context"
	"testing"
	"time"

	"github.com/valyala/fasthttp"
	zaphttp "github.com/zap-proto/http"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

// TestZapReceiverAllSignals proves that OTLP traces, logs, and metrics flow
// end-to-end over the ZAP wire (zap-proto/http) into the pipeline consumers —
// no http/grpc anywhere: the client uses zaphttp.Transport.
func TestZapReceiverAllSignals(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig().(*Config)
	cfg.Endpoint = "127.0.0.1:0"

	tracesSink := new(consumertest.TracesSink)
	logsSink := new(consumertest.LogsSink)
	metricsSink := new(consumertest.MetricsSink)

	set := receivertest.NewNopSettings(typeStr)
	if _, err := factory.CreateTraces(context.Background(), set, cfg, tracesSink); err != nil {
		t.Fatalf("create traces: %v", err)
	}
	if _, err := factory.CreateLogs(context.Background(), set, cfg, logsSink); err != nil {
		t.Fatalf("create logs: %v", err)
	}
	rcv, err := factory.CreateMetrics(context.Background(), set, cfg, metricsSink)
	if err != nil {
		t.Fatalf("create metrics: %v", err)
	}

	if err := rcv.Start(context.Background(), componenttest.NewNopHost()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = rcv.Shutdown(context.Background()) }()

	addr := rcv.(*zapReceiver).addrForTest()
	if addr == "" {
		t.Fatal("no listen addr")
	}
	tr := zaphttp.Dial("tcp", addr)
	defer tr.CloseIdleConnections()

	post := func(path string, body []byte) int {
		req := fasthttp.AcquireRequest()
		resp := fasthttp.AcquireResponse()
		defer fasthttp.ReleaseRequest(req)
		defer fasthttp.ReleaseResponse(resp)
		req.Header.SetMethod(fasthttp.MethodPost)
		req.SetRequestURI("http://zap" + path)
		req.Header.SetHost(addr)
		req.Header.SetContentType("application/x-protobuf")
		req.SetBody(body)
		if err := tr.Do(req, resp); err != nil {
			t.Fatalf("zap Do %s: %v", path, err)
		}
		return resp.StatusCode()
	}

	// traces
	traceBody, err := ptraceotlp.NewExportRequestFromTraces(oneSpan()).MarshalProto()
	if err != nil {
		t.Fatal(err)
	}
	if code := post(pathTraces, traceBody); code != 200 {
		t.Fatalf("traces status = %d", code)
	}

	// logs
	logBody, err := plogotlp.NewExportRequestFromLogs(oneLog()).MarshalProto()
	if err != nil {
		t.Fatal(err)
	}
	if code := post(pathLogs, logBody); code != 200 {
		t.Fatalf("logs status = %d", code)
	}

	// metrics
	metricBody, err := pmetricotlp.NewExportRequestFromMetrics(oneMetric()).MarshalProto()
	if err != nil {
		t.Fatal(err)
	}
	if code := post(pathMetrics, metricBody); code != 200 {
		t.Fatalf("metrics status = %d", code)
	}

	// The consumers must have received exactly what we shipped over ZAP.
	waitFor(t, func() bool { return tracesSink.SpanCount() == 1 }, "1 span")
	waitFor(t, func() bool { return logsSink.LogRecordCount() == 1 }, "1 log record")
	waitFor(t, func() bool { return metricsSink.DataPointCount() == 1 }, "1 metric data point")
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func oneSpan() ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "zap-test")
	sp := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	sp.SetName("zap-span")
	sp.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	sp.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})
	return td
}

func oneLog() plog.Logs {
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "zap-test")
	lr := rl.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()
	lr.Body().SetStr("hello over zap")
	return ld
}

func oneMetric() pmetric.Metrics {
	md := pmetric.NewMetrics()
	rm := md.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", "zap-test")
	m := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("zap.counter")
	dp := m.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.SetIntValue(1)
	return md
}
