// Brought in as is from opentelemetry-collector-contrib

package noop

import (
	"go.opentelemetry.io/collector/component"

	o11ylogspipelinestanzaoperator "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator"
	o11ystanzahelper "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/helper"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator"
)

const operatorType = "noop"

func init() {
	o11ylogspipelinestanzaoperator.Register(operatorType, func() operator.Builder { return NewConfig() })
}

// NewConfig creates a new noop operator config with default values
func NewConfig() *Config {
	return NewConfigWithID(operatorType)
}

// NewConfigWithID creates a new noop operator config with default values
func NewConfigWithID(operatorID string) *Config {
	return &Config{
		TransformerConfig: o11ystanzahelper.NewTransformerConfig(operatorID, operatorType),
	}
}

// Config is the configuration of a noop operator.
type Config struct {
	o11ystanzahelper.TransformerConfig `mapstructure:",squash"`
}

// Build will build a noop operator.
func (c Config) Build(set component.TelemetrySettings) (operator.Operator, error) {
	transformerOperator, err := c.TransformerConfig.Build(set)
	if err != nil {
		return nil, err
	}

	return &Transformer{
		TransformerOperator: transformerOperator,
	}, nil
}
