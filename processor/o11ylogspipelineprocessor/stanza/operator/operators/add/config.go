// Brought in as is from opentelemetry-collector-contrib

package add

import (
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/component"

	o11ylogspipelinestanzaoperator "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator"
	o11ystanzahelper "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/helper"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/entry"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator"
)

const operatorType = "add"

func init() {
	o11ylogspipelinestanzaoperator.Register(operatorType, func() operator.Builder { return NewConfig() })
}

// NewConfig creates a new add operator config with default values
func NewConfig() *Config {
	return NewConfigWithID(operatorType)
}

// NewConfigWithID creates a new add operator config with default values
func NewConfigWithID(operatorID string) *Config {
	return &Config{
		TransformerConfig: o11ystanzahelper.NewTransformerConfig(operatorID, operatorType),
	}
}

// Config is the configuration of an add operator
type Config struct {
	o11ystanzahelper.TransformerConfig `mapstructure:",squash"`
	Field                                entry.Field `mapstructure:"field"`
	Value                                any         `mapstructure:"value,omitempty"`
}

// Build will build an add operator from the supplied configuration
func (c Config) Build(set component.TelemetrySettings) (operator.Operator, error) {
	transformerOperator, err := c.TransformerConfig.Build(set)
	if err != nil {
		return nil, err
	}

	addOperator := &Transformer{
		TransformerOperator: transformerOperator,
		Field:               c.Field,
	}
	strVal, ok := c.Value.(string)
	if !ok || !isExpr(strVal) {
		addOperator.Value = c.Value
		return addOperator, nil
	}
	exprStr := strings.TrimPrefix(strVal, "EXPR(")
	exprStr = strings.TrimSuffix(exprStr, ")")

	compiled, hasBodyFieldRef, err := o11ystanzahelper.ExprCompile(exprStr)
	if err != nil {
		return nil, fmt.Errorf("failed to compile expression '%s': %w", c.IfExpr, err)
	}

	addOperator.program = compiled
	addOperator.valueExprHasBodyFieldRef = hasBodyFieldRef
	return addOperator, nil
}
