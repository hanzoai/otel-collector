package zapexporter

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"

	// Registers the QUIC transport factory at init so Config.Transport "quic" is
	// actually available; without it luxfi/zap returns ErrTransportUnavailable.
	// QUIC is the REMOTE hop: its TLS 1.3 negotiates X25519MLKEM768 (X-Wing) by
	// default on Go 1.26, so cross-machine telemetry is quantum-secure.
	_ "github.com/luxfi/zap/quic"
)

var typeStr = component.MustNewType("zap")

const (
	defaultEndpoint = "cloud.hanzo.svc:4318"
	stability       = component.StabilityLevelBeta
)

// NewFactory returns the ZAP-native OTLP exporter factory.
func NewFactory() exporter.Factory {
	return exporter.NewFactory(
		typeStr,
		createDefaultConfig,
		exporter.WithTraces(createTraces, stability),
		exporter.WithLogs(createLogs, stability),
		exporter.WithMetrics(createMetrics, stability),
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		Endpoint:         defaultEndpoint,
		TimeoutConfig:    exporterhelper.NewDefaultTimeoutConfig(),
		QueueBatchConfig: configoptional.Some(exporterhelper.NewDefaultQueueConfig()),
		BackOffConfig:    configretry.NewDefaultBackOffConfig(),
	}
}

func createTraces(ctx context.Context, set exporter.Settings, cfg component.Config) (exporter.Traces, error) {
	c := cfg.(*Config)
	e := newExporter(set, c)
	return exporterhelper.NewTraces(ctx, set, cfg, e.pushTraces,
		exporterhelper.WithStart(e.Start),
		exporterhelper.WithShutdown(e.Shutdown),
		exporterhelper.WithTimeout(c.TimeoutConfig),
		exporterhelper.WithQueue(c.QueueBatchConfig),
		exporterhelper.WithRetry(c.BackOffConfig),
	)
}

func createLogs(ctx context.Context, set exporter.Settings, cfg component.Config) (exporter.Logs, error) {
	c := cfg.(*Config)
	e := newExporter(set, c)
	return exporterhelper.NewLogs(ctx, set, cfg, e.pushLogs,
		exporterhelper.WithStart(e.Start),
		exporterhelper.WithShutdown(e.Shutdown),
		exporterhelper.WithTimeout(c.TimeoutConfig),
		exporterhelper.WithQueue(c.QueueBatchConfig),
		exporterhelper.WithRetry(c.BackOffConfig),
	)
}

func createMetrics(ctx context.Context, set exporter.Settings, cfg component.Config) (exporter.Metrics, error) {
	c := cfg.(*Config)
	e := newExporter(set, c)
	return exporterhelper.NewMetrics(ctx, set, cfg, e.pushMetrics,
		exporterhelper.WithStart(e.Start),
		exporterhelper.WithShutdown(e.Shutdown),
		exporterhelper.WithTimeout(c.TimeoutConfig),
		exporterhelper.WithQueue(c.QueueBatchConfig),
		exporterhelper.WithRetry(c.BackOffConfig),
	)
}
