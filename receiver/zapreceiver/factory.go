package zapreceiver

import (
	"context"
	"sync"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
)

// typeStr is the config `receivers:` key for this component.
var typeStr = component.MustNewType("zap")

// defaultEndpoint is the ZAP telemetry port. It is deliberately NOT 4317
// (OTLP-gRPC) or 4318 (OTLP-HTTP): those remain the interop OTLP endpoints,
// while :4319 carries the ZAP-native wire that Hanzo services use.
const defaultEndpoint = "0.0.0.0:4319"

const stability = component.StabilityLevelBeta

// NewFactory returns a receiver.Factory for the ZAP-native OTLP receiver. A
// single listener serves all three signals; the signal factories share one
// receiver instance keyed by config, so `traces`, `logs`, and `metrics`
// pipelines that reference the same receiver bind exactly one port.
func NewFactory() receiver.Factory {
	return receiver.NewFactory(
		typeStr,
		createDefaultConfig,
		receiver.WithTraces(createTraces, stability),
		receiver.WithLogs(createLogs, stability),
		receiver.WithMetrics(createMetrics, stability),
	)
}

func createDefaultConfig() component.Config {
	return &Config{Endpoint: defaultEndpoint}
}

var (
	recvMu sync.Mutex
	recvs  = map[*Config]*zapReceiver{}
)

func ensure(set receiver.Settings, cfg *Config) (*zapReceiver, error) {
	recvMu.Lock()
	defer recvMu.Unlock()
	r := recvs[cfg]
	if r == nil {
		var err error
		r, err = newReceiver(set, cfg)
		if err != nil {
			return nil, err
		}
		recvs[cfg] = r
	}
	return r, nil
}

func createTraces(_ context.Context, set receiver.Settings, cfg component.Config, next consumer.Traces) (receiver.Traces, error) {
	r, err := ensure(set, cfg.(*Config))
	if err != nil {
		return nil, err
	}
	r.traces = next
	return r, nil
}

func createLogs(_ context.Context, set receiver.Settings, cfg component.Config, next consumer.Logs) (receiver.Logs, error) {
	r, err := ensure(set, cfg.(*Config))
	if err != nil {
		return nil, err
	}
	r.logs = next
	return r, nil
}

func createMetrics(_ context.Context, set receiver.Settings, cfg component.Config, next consumer.Metrics) (receiver.Metrics, error) {
	r, err := ensure(set, cfg.(*Config))
	if err != nil {
		return nil, err
	}
	r.metrics = next
	return r, nil
}
