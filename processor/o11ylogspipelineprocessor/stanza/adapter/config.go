package o11ylogspipelinestanzaadapter

import (
	o11ylogspipelinestanzaoperator "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator"
)

type BaseConfig struct {
	// Using our own version of Config allows using a dedicated registry of stanza ops for logs pipelines.
	Operators []o11ylogspipelinestanzaoperator.Config `mapstructure:"operators"`
}
