package zapreceiver

import (
	"errors"
	"strings"
)

// Config configures the ZAP-native OTLP receiver.
type Config struct {
	// Endpoint is what the ZAP listener binds. The wire is ZAP frames
	// (zap-proto/http), not OTLP-HTTP/gRPC.
	//
	// A host:port binds TCP. A filesystem path, or a unix:// URL, binds a Unix
	// socket instead — which is how a process on the same host reaches the
	// agent without a port. Default "0.0.0.0:4319".
	Endpoint string `mapstructure:"endpoint"`
}

// Network returns the net.Listen network and address for Endpoint. Anything
// that looks like a path is a socket; everything else is TCP.
func (c *Config) Network() (network, address string) {
	if strings.HasPrefix(c.Endpoint, "unix://") {
		return "unix", strings.TrimPrefix(c.Endpoint, "unix://")
	}
	if strings.HasPrefix(c.Endpoint, "/") || strings.HasPrefix(c.Endpoint, "./") {
		return "unix", c.Endpoint
	}
	return "tcp", c.Endpoint
}

// Validate implements component.ConfigValidator.
func (c *Config) Validate() error {
	if c.Endpoint == "" {
		return errors.New("zapreceiver: endpoint must be set")
	}
	return nil
}
