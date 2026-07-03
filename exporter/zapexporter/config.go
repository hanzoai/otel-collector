package zapexporter

import (
	"errors"

	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

// Config configures the ZAP-native OTLP exporter.
type Config struct {
	// Endpoint is the host:port of the target collector's ZAP receiver
	// (zap-proto/http wire). e.g. "otel-collector.hanzo.svc:4319".
	Endpoint string `mapstructure:"endpoint"`

	TimeoutConfig    exporterhelper.TimeoutConfig                             `mapstructure:",squash"`
	BackOffConfig    configretry.BackOffConfig                                `mapstructure:"retry_on_failure"`
	QueueBatchConfig configoptional.Optional[exporterhelper.QueueBatchConfig] `mapstructure:"sending_queue"`
}

// Validate implements component.ConfigValidator.
func (c *Config) Validate() error {
	if c.Endpoint == "" {
		return errors.New("zapexporter: endpoint must be set")
	}
	return nil
}
