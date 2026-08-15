package zapreceiver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	ln   net.Listener
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
	network, address := r.cfg.Network()
	if network == "unix" {
		// sun_path is 104 bytes on darwin/BSD and 108 on linux. Past that the
		// bind fails with "invalid argument", which says nothing about length.
		if len(address) > 100 {
			return fmt.Errorf("zapreceiver: socket path is %d bytes, over the ~100 the kernel allows: %s", len(address), address)
		}
		if dir := filepath.Dir(address); dir != "." && !strings.HasPrefix(address, "@") {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("zapreceiver: mkdir %s: %w", dir, err)
			}
		}
		if err := clearDeadSocket(address); err != nil {
			return fmt.Errorf("zapreceiver: %w", err)
		}
	}
	ln, err := net.Listen(network, address)
	if err != nil {
		return fmt.Errorf("zapreceiver: listen %s %s: %w", network, address, err)
	}
	if network == "unix" {
		// The emitting process is not always the same user as the collector.
		if err := os.Chmod(address, 0o666); err != nil {
			return fmt.Errorf("zapreceiver: chmod %s: %w", address, err)
		}
	}
	srv := &zaphttp.Server{Handler: r.handle}
	r.addr = ln.Addr().String()
	r.ln = ln
	r.srv = srv

	r.wg.Add(1)
	// srv and ln are captured: Shutdown clears the fields, and the goroutine
	// must not read them while it is stopping.
	go func() {
		defer r.wg.Done()
		serveErr := srv.Serve(ln)
		if serveErr == nil || errors.Is(serveErr, net.ErrClosed) {
			return
		}
		r.settings.Logger.Error("zapreceiver serve stopped", zap.Error(serveErr))
		componentstatus.ReportStatus(host, componentstatus.NewFatalErrorEvent(serveErr))
	}()
	r.settings.Logger.Info("ZAP-native OTLP receiver started (wire=zap, not http/grpc)",
		zap.String("endpoint", r.addr))
	return nil
}

// clearDeadSocket removes a socket a previous process left behind, which would
// otherwise fail the bind with EADDRINUSE while nothing is listening. A socket
// something IS listening on is left alone, so two collectors cannot silently
// take each other's address.
func clearDeadSocket(path string) error {
	if strings.HasPrefix(path, "@") {
		return nil // abstract sockets have no filesystem entry
	}
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if fi.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%s exists and is not a socket", path)
	}
	if c, err := net.DialTimeout("unix", path, 200*time.Millisecond); err == nil {
		_ = c.Close()
		return fmt.Errorf("%s is already served by a live receiver", path)
	}
	return os.Remove(path)
}

// Shutdown stops the listener and waits for the serve loop to return.
func (r *zapReceiver) Shutdown(context.Context) error {
	r.mu.Lock()
	srv, ln := r.srv, r.ln
	r.srv, r.ln = nil, nil
	r.mu.Unlock()
	if srv == nil {
		return nil
	}
	// Close the listener, not only the server: Serve blocks in Accept on a
	// listener the server did not open, so closing the server alone leaves the
	// accept loop parked and wg.Wait below never returns.
	if ln != nil {
		_ = ln.Close()
	}
	err := srv.Close()
	r.wg.Wait()
	// The server closes the same listener, so a clean stop reports it as
	// already closed.
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
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
