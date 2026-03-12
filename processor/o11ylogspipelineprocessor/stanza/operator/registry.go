// Stanza operators registry dedicated to O11y logs pipelines

package o11ylogspipelinestanzaoperator

import "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/operator"

var O11yStanzaOperatorsRegistry = operator.NewRegistry()

// Register will register an operator in the default registry
func Register(operatorType string, newBuilder func() operator.Builder) {
	O11yStanzaOperatorsRegistry.Register(operatorType, newBuilder)
}

// Lookup looks up a given operator type.Its second return value will
// be false if no builder is registered for that type.
func Lookup(configType string) (func() operator.Builder, bool) {
	return O11yStanzaOperatorsRegistry.Lookup(configType)
}
