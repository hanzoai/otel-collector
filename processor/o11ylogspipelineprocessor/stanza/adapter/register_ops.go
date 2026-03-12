// Register copies of stanza operators dedicated to o11y logs pipelines
package o11ylogspipelinestanzaadapter

import (
	_ "github.com/hanzoai/otel-collector/pkg/parser/grok"
	_ "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/operators/add"
	_ "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/operators/copy"
	_ "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/operators/json"
	_ "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/operators/move"
	_ "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/operators/noop"
	_ "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/operators/normalize"
	_ "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/operators/regex"
	_ "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/operators/remove"
	_ "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/operators/router"
	_ "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/operators/severity"
	_ "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/operators/time"
	_ "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/operators/trace"
)
