package zapexporter

import (
	"context"
	"fmt"

	"github.com/valyala/fasthttp"
	zaphttp "github.com/zap-proto/http"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"go.uber.org/zap"
)

const contentTypeProto = "application/x-protobuf"

type zapExporter struct {
	cfg      *Config
	settings exporter.Settings
	tr       *zaphttp.Transport
}

func newExporter(set exporter.Settings, cfg *Config) *zapExporter {
	return &zapExporter{cfg: cfg, settings: set}
}

// Start dials nothing eagerly; the pooled ZAP transport connects lazily.
func (e *zapExporter) Start(context.Context, component.Host) error {
	e.tr = zaphttp.NewTransport(e.cfg.Endpoint)
	if t := e.cfg.TimeoutConfig.Timeout; t > 0 {
		e.tr.SetReadTimeout(t)
	}
	e.settings.Logger.Info("ZAP-native OTLP exporter ready (wire=zap, not http/grpc)",
		zap.String("endpoint", e.cfg.Endpoint))
	return nil
}

func (e *zapExporter) Shutdown(context.Context) error {
	if e.tr != nil {
		e.tr.CloseIdleConnections()
	}
	return nil
}

func (e *zapExporter) pushTraces(ctx context.Context, td ptrace.Traces) error {
	body, err := ptraceotlp.NewExportRequestFromTraces(td).MarshalProto()
	if err != nil {
		return consumererror.NewPermanent(err)
	}
	return e.post(ctx, "/v1/traces", body)
}

func (e *zapExporter) pushLogs(ctx context.Context, ld plog.Logs) error {
	body, err := plogotlp.NewExportRequestFromLogs(ld).MarshalProto()
	if err != nil {
		return consumererror.NewPermanent(err)
	}
	return e.post(ctx, "/v1/logs", body)
}

func (e *zapExporter) pushMetrics(ctx context.Context, md pmetric.Metrics) error {
	body, err := pmetricotlp.NewExportRequestFromMetrics(md).MarshalProto()
	if err != nil {
		return consumererror.NewPermanent(err)
	}
	return e.post(ctx, "/v1/metrics", body)
}

// post sends one OTLP payload over the ZAP wire. 5xx is returned as-is
// (retryable by exporterhelper); 4xx is wrapped permanent.
func (e *zapExporter) post(_ context.Context, path string, body []byte) error {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.Header.SetMethod(fasthttp.MethodPost)
	req.SetRequestURI("http://zap" + path)
	req.Header.SetHost(e.cfg.Endpoint)
	req.Header.SetContentType(contentTypeProto)
	req.SetBody(body)

	if err := e.tr.Do(req, resp); err != nil {
		return fmt.Errorf("zapexporter: zap send %s: %w", path, err)
	}
	code := resp.StatusCode()
	if code/100 == 2 {
		return nil
	}
	err := fmt.Errorf("zapexporter: %s -> %d: %s", path, code, resp.Body())
	if code >= 500 {
		return err
	}
	return consumererror.NewPermanent(err)
}
