// Brought in as is from opentelemetry-collector-contrib

package move

import (
	"fmt"

	"go.opentelemetry.io/collector/component"

	o11ylogspipelinestanzaoperator "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator"
	o11ystanzahelper "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/helper"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/entry"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator"
)

const operatorType = "move"

func init() {
	o11ylogspipelinestanzaoperator.Register(operatorType, func() operator.Builder { return NewConfig() })
}

// NewConfig creates a new move operator config with default values
func NewConfig() *Config {
	return NewConfigWithID(operatorType)
}

// NewConfigWithID creates a new move operator config with default values
func NewConfigWithID(operatorID string) *Config {
	return &Config{
		TransformerConfig: o11ystanzahelper.NewTransformerConfig(operatorID, operatorType),
	}
}

// Config is the configuration of a move operator
type Config struct {
	o11ystanzahelper.TransformerConfig `mapstructure:",squash"`
	From                                 entry.Field `mapstructure:"from"`
	To                                   entry.Field `mapstructure:"to"`
}

// Build will build a Move operator from the supplied configuration
func (c Config) Build(set component.TelemetrySettings) (operator.Operator, error) {
	transformerOperator, err := c.TransformerConfig.Build(set)
	if err != nil {
		return nil, err
	}

	if c.To == entry.NewNilField() || c.From == entry.NewNilField() {
		return nil, fmt.Errorf("move: missing to or from field")
	}

	return &Transformer{
		TransformerOperator: transformerOperator,
		From:                c.From,
		To:                  c.To,
	}, nil
}
