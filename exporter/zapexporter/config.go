package zapexporter

import (
	"errors"
	"fmt"

	"github.com/luxfi/zap"

	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

// Config configures the OTLZ exporter.
//
// TRANSPORT BY LOCALITY, not by caller choice. The address SHAPE picks the
// transport, which is luxfi/zap's own rule (Network(addr): a path is "unix",
// anything else is host:port TCP) and it is used by the listener and the dialer
// alike, so a node can never bind one transport and be dialled on another:
//
//	same process  → no exporter at all; call the consumer directly
//	same machine  → Endpoint is a socket PATH  ("/run/hanzo/o11y-logs.sock")
//	                → UDS, no kernel network stack, no port, no TLS to get wrong
//	remote host   → Endpoint is host:port + Transport "quic"
//	                → QUIC, whose TLS 1.3 negotiates X25519MLKEM768 (X-Wing) by
//	                  default on Go 1.26, so the session key is quantum-secure
//	                  without any per-caller crypto configuration
//
// A cross-machine hop on plain TCP is the case worth naming: the node agent
// ships every pod's logs from 18 nodes to one collector pod across the cluster
// network, so it carries other services' log bodies — which is exactly the
// traffic that should not be readable in transit.
type Config struct {
	// Endpoint is where the collector's OTLZ receiver listens. A path
	// ("/run/hanzo/o11y-logs.sock", "@o11y-logs" for Linux's abstract
	// namespace) selects UDS; "host:port" selects a network transport.
	Endpoint string `mapstructure:"endpoint"`

	// Transport picks the network transport for a host:port endpoint:
	// "tcp" (default) or "quic". Ignored for a UDS path, where the socket IS
	// the transport. "quic" additionally requires the QUIC factory to be linked
	// into the binary — factory.go imports it, so any build that includes this
	// exporter has it.
	Transport string `mapstructure:"transport"`

	TimeoutConfig    exporterhelper.TimeoutConfig                             `mapstructure:",squash"`
	BackOffConfig    configretry.BackOffConfig                                `mapstructure:"retry_on_failure"`
	QueueBatchConfig configoptional.Optional[exporterhelper.QueueBatchConfig] `mapstructure:"sending_queue"`
}

// Validate implements component.ConfigValidator.
func (c *Config) Validate() error {
	if c.Endpoint == "" {
		return errors.New("zapexporter: endpoint must be set")
	}
	switch c.Transport {
	case "", "tcp", "quic":
	default:
		return fmt.Errorf("zapexporter: transport %q must be \"tcp\" or \"quic\"", c.Transport)
	}
	if c.Transport == "quic" && isSocketPath(c.Endpoint) {
		// Caught here rather than silently ignored: someone who asked for QUIC
		// and got a unix socket should learn it from config validation, not from
		// wondering why the traffic is unencrypted.
		return fmt.Errorf("zapexporter: transport %q is meaningless for the socket path %q — a UDS hop never leaves the machine", c.Transport, c.Endpoint)
	}
	return nil
}

// isSocketPath asks luxfi/zap, which owns the rule.
func isSocketPath(addr string) bool { return zap.Network(addr) == "unix" }
