package zapreceiver

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/valyala/fasthttp"
	zaphttp "github.com/zap-proto/http"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/receiverhelper"
	"go.uber.org/zap"
)

// transport is the ObsReport transport label — proof, in the collector's own
// self-telemetry, that these signals arrived over ZAP and not http/grpc.
const transport = "zap"

const (
	pathTraces  = "/v1/traces"
	pathLogs    = "/v1/logs"
	pathMetrics = "/v1/metrics"
)

// zapReceiver terminates OTLP payloads carried over the ZAP-HTTP wire.
type zapReceiver struct {
	settings receiver.Settings
	cfg      *Config

	traces  consumer.Traces
	logs    consumer.Logs
	metrics consumer.Metrics

	obsT *receiverhelper.ObsReport
	obsL *receiverhelper.ObsReport
	obsM *receiverhelper.ObsReport

	mu   sync.Mutex
	srv  *zaphttp.Server
	addr string
	wg   sync.WaitGroup
}

func newReceiver(set receiver.Settings, cfg *Config) (*zapReceiver, error) {
	mkObs := func() (*receiverhelper.ObsReport, error) {
		return receiverhelper.NewObsReport(receiverhelper.ObsReportSettings{
			ReceiverID:             set.ID,
			Transport:              transport,
			ReceiverCreateSettings: set,
		})
	}
	obsT, err := mkObs()
	if err != nil {
		return nil, err
	}
	obsL, err := mkObs()
	if err != nil {
		return nil, err
	}
	obsM, err := mkObs()
	if err != nil {
		return nil, err
	}
	return &zapReceiver{settings: set, cfg: cfg, obsT: obsT, obsL: obsL, obsM: obsM}, nil
}

// Start binds the ZAP-HTTP listener and serves the OTLP-over-ZAP handler.
func (r *zapReceiver) Start(_ context.Context, host component.Host) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.srv != nil {
		return nil
	}
	ln, err := net.Listen("tcp", r.cfg.Endpoint)
	if err != nil {
		return fmt.Errorf("zapreceiver: listen %s: %w", r.cfg.Endpoint, err)
	}
	r.addr = ln.Addr().String()
	r.srv = &zaphttp.Server{Handler: r.handle}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		if serveErr := r.srv.Serve(ln); serveErr != nil {
			r.settings.Logger.Error("zapreceiver serve stopped", zap.Error(serveErr))
			componentstatus.ReportStatus(host, componentstatus.NewFatalErrorEvent(serveErr))
		}
	}()
	r.settings.Logger.Info("ZAP-native OTLP receiver started (wire=zap, not http/grpc)",
		zap.String("endpoint", r.addr))
	return nil
}

// Shutdown stops the ZAP-HTTP listener.
func (r *zapReceiver) Shutdown(context.Context) error {
	r.mu.Lock()
	srv := r.srv
	r.mu.Unlock()
	if srv == nil {
		return nil
	}
	err := srv.Close()
	r.wg.Wait()
	return err
}

// addrForTest exposes the resolved listen address (used by tests when the
// configured endpoint is :0).
func (r *zapReceiver) addrForTest() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.addr
}

// handle is the fasthttp.RequestHandler dispatched by zaphttp for each ZAP
// frame. It routes the standard OTLP/HTTP signal paths.
func (r *zapReceiver) handle(ctx *fasthttp.RequestCtx) {
	if !ctx.IsPost() {
		reject(ctx, fasthttp.StatusMethodNotAllowed, "zapreceiver: POST required")
		return
	}
	switch string(ctx.Path()) {
	case pathTraces:
		r.handleTraces(ctx)
	case pathLogs:
		r.handleLogs(ctx)
	case pathMetrics:
		r.handleMetrics(ctx)
	default:
		reject(ctx, fasthttp.StatusNotFound, "zapreceiver: unknown path "+string(ctx.Path()))
	}
}

func isJSON(ctx *fasthttp.RequestCtx) bool {
	return bytes.Contains(ctx.Request.Header.ContentType(), []byte("json"))
}

func (r *zapReceiver) handleTraces(ctx *fasthttp.RequestCtx) {
	if r.traces == nil {
		reject(ctx, fasthttp.StatusNotFound, "zapreceiver: traces pipeline not configured")
		return
	}
	req := ptraceotlp.NewExportRequest()
	if err := unmarshal(ctx, func(b []byte) error { return req.UnmarshalProto(b) }, func(b []byte) error { return req.UnmarshalJSON(b) }); err != nil {
		reject(ctx, fasthttp.StatusBadRequest, "zapreceiver: decode traces: "+err.Error())
		return
	}
	td := req.Traces()
	octx := r.obsT.StartTracesOp(context.Background())
	n := td.SpanCount()
	cErr := r.traces.ConsumeTraces(octx, td)
	r.obsT.EndTracesOp(octx, transport, n, cErr)
	if cErr != nil {
		reject(ctx, fasthttp.StatusInternalServerError, "zapreceiver: consume traces: "+cErr.Error())
		return
	}
	resp := ptraceotlp.NewExportResponse()
	writeResponse(ctx, func() ([]byte, error) { return resp.MarshalProto() }, func() ([]byte, error) { return resp.MarshalJSON() })
}

func (r *zapReceiver) handleLogs(ctx *fasthttp.RequestCtx) {
	if r.logs == nil {
		reject(ctx, fasthttp.StatusNotFound, "zapreceiver: logs pipeline not configured")
		return
	}
	req := plogotlp.NewExportRequest()
	if err := unmarshal(ctx, func(b []byte) error { return req.UnmarshalProto(b) }, func(b []byte) error { return req.UnmarshalJSON(b) }); err != nil {
		reject(ctx, fasthttp.StatusBadRequest, "zapreceiver: decode logs: "+err.Error())
		return
	}
	ld := req.Logs()
	octx := r.obsL.StartLogsOp(context.Background())
	n := ld.LogRecordCount()
	cErr := r.logs.ConsumeLogs(octx, ld)
	r.obsL.EndLogsOp(octx, transport, n, cErr)
	if cErr != nil {
		reject(ctx, fasthttp.StatusInternalServerError, "zapreceiver: consume logs: "+cErr.Error())
		return
	}
	resp := plogotlp.NewExportResponse()
	writeResponse(ctx, func() ([]byte, error) { return resp.MarshalProto() }, func() ([]byte, error) { return resp.MarshalJSON() })
}

func (r *zapReceiver) handleMetrics(ctx *fasthttp.RequestCtx) {
	if r.metrics == nil {
		reject(ctx, fasthttp.StatusNotFound, "zapreceiver: metrics pipeline not configured")
		return
	}
	req := pmetricotlp.NewExportRequest()
	if err := unmarshal(ctx, func(b []byte) error { return req.UnmarshalProto(b) }, func(b []byte) error { return req.UnmarshalJSON(b) }); err != nil {
		reject(ctx, fasthttp.StatusBadRequest, "zapreceiver: decode metrics: "+err.Error())
		return
	}
	md := req.Metrics()
	octx := r.obsM.StartMetricsOp(context.Background())
	n := md.DataPointCount()
	cErr := r.metrics.ConsumeMetrics(octx, md)
	r.obsM.EndMetricsOp(octx, transport, n, cErr)
	if cErr != nil {
		reject(ctx, fasthttp.StatusInternalServerError, "zapreceiver: consume metrics: "+cErr.Error())
		return
	}
	resp := pmetricotlp.NewExportResponse()
	writeResponse(ctx, func() ([]byte, error) { return resp.MarshalProto() }, func() ([]byte, error) { return resp.MarshalJSON() })
}

// unmarshal decodes the request body as OTLP protobuf (default) or JSON.
func unmarshal(ctx *fasthttp.RequestCtx, proto func([]byte) error, jsn func([]byte) error) error {
	body := ctx.PostBody()
	if isJSON(ctx) {
		return jsn(body)
	}
	return proto(body)
}

// writeResponse emits the OTLP ExportResponse in the request's encoding.
func writeResponse(ctx *fasthttp.RequestCtx, proto func() ([]byte, error), jsn func() ([]byte, error)) {
	var (
		out []byte
		err error
	)
	if isJSON(ctx) {
		out, err = jsn()
		ctx.SetContentType("application/json")
	} else {
		out, err = proto()
		ctx.SetContentType("application/x-protobuf")
	}
	if err != nil {
		reject(ctx, fasthttp.StatusInternalServerError, "zapreceiver: marshal response: "+err.Error())
		return
	}
	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetBody(out)
}

func reject(ctx *fasthttp.RequestCtx, status int, msg string) {
	ctx.SetStatusCode(status)
	ctx.SetContentType("text/plain; charset=utf-8")
	ctx.SetBodyString(msg)
}
