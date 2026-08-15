package zapreceiver

import (
	"errors"

	"github.com/luxfi/zap"
)

// Config configures the ZAP-native OTLP receiver.
type Config struct {
	// Endpoint is what the ZAP listener binds. The wire is ZAP frames
	// (zap-proto/http), not OTLP-HTTP/gRPC.
	//
	// A host:port binds TCP; a path binds a Unix socket, which is how a
	// process on the same host reaches the agent without a port. Default
	// "0.0.0.0:4319".
	Endpoint string `mapstructure:"endpoint"`
}

// Network returns the net.Listen network and address for Endpoint, from
// luxfi/zap's rule — the one the dialer uses too, so a node cannot bind one
// transport and be dialled on another.
func (c *Config) Network() (network, address string) {
	return zap.Network(c.Endpoint), c.Endpoint
}

// Validate implements component.ConfigValidator.
func (c *Config) Validate() error {
	if c.Endpoint == "" {
		return errors.New("zapreceiver: endpoint must be set")
	}
	return nil
}
