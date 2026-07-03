package zapreceiver

import "errors"

// Config configures the ZAP-native OTLP receiver.
type Config struct {
	// Endpoint is the host:port the ZAP-HTTP listener binds. The wire is ZAP
	// frames (zap-proto/http), NOT OTLP-HTTP/gRPC. Default "0.0.0.0:4319".
	Endpoint string `mapstructure:"endpoint"`
}

// Validate implements component.ConfigValidator.
func (c *Config) Validate() error {
	if c.Endpoint == "" {
		return errors.New("zapreceiver: endpoint must be set")
	}
	return nil
}
