// Brought in as is from opentelemetry-collector-contrib

package time

import (
	"context"

	o11ystanzahelper "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/helper"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/entry"
)

// Parser is an operator that parses time from a field to an entry.
type Parser struct {
	o11ystanzahelper.TransformerOperator
	o11ystanzahelper.TimeParser
}

// Process will parse time from an entry.
func (p *Parser) Process(ctx context.Context, entry *entry.Entry) error {
	return p.ProcessWith(ctx, entry, p.TimeParser.Parse)
}

func (p *Parser) ProcessBatch(ctx context.Context, entries []*entry.Entry) error {
	return p.ProcessBatchWith(ctx, entries, p.Process)
}
