// Brought in as is from opentelemetry-collector-contrib

package time

import (
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"

	o11ylogspipelinestanzaoperator "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator"
	o11ystanzahelper "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/helper"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator"
)

const operatorType = "time_parser"

func init() {
	o11ylogspipelinestanzaoperator.Register(operatorType, func() operator.Builder { return NewConfig() })
}

// NewConfig creates a new time parser config with default values
func NewConfig() *Config {
	return NewConfigWithID(operatorType)
}

// NewConfigWithID creates a new time parser config with default values
func NewConfigWithID(operatorID string) *Config {
	return &Config{
		TransformerConfig: o11ystanzahelper.NewTransformerConfig(operatorID, operatorType),
		TimeParser:        o11ystanzahelper.NewTimeParser(),
	}
}

// Config is the configuration of a time parser operator.
type Config struct {
	o11ystanzahelper.TransformerConfig `mapstructure:",squash"`
	o11ystanzahelper.TimeParser        `mapstructure:",omitempty,squash"`
}

func (c *Config) Unmarshal(component *confmap.Conf) error {
	return component.Unmarshal(c, confmap.WithIgnoreUnused())
}

// Build will build a time parser operator.
func (c Config) Build(set component.TelemetrySettings) (operator.Operator, error) {
	transformerOperator, err := c.TransformerConfig.Build(set)
	if err != nil {
		return nil, err
	}

	if err := c.TimeParser.Validate(); err != nil {
		return nil, err
	}

	return &Parser{
		TransformerOperator: transformerOperator,
		TimeParser:          c.TimeParser,
	}, nil
}
