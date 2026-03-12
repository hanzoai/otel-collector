// Brought in as is from opentelemetry-collector-contrib
package severity

import (
	"path/filepath"
	"testing"

	o11ystanzaentry "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/entry"
	"github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/operatortest"
)

func TestUnmarshal(t *testing.T) {
	operatortest.ConfigUnmarshalTests{
		DefaultConfig: NewConfig(),
		TestsFile:     filepath.Join(".", "testdata", "config.yaml"),
		Tests: []operatortest.ConfigUnmarshalTest{
			{
				Name:   "default",
				Expect: NewConfig(),
			},
			{
				Name: "on_error_drop",
				Expect: func() *Config {
					cfg := NewConfig()
					cfg.OnError = "drop"
					return cfg
				}(),
			},
			{
				Name: "parse_from_simple",
				Expect: func() *Config {
					cfg := NewConfig()
					from := o11ystanzaentry.Field{FieldInterface: o11ystanzaentry.NewBodyField("from")}
					cfg.ParseFrom = &from
					return cfg
				}(),
			},
			{
				Name: "parse_with_preset",
				Expect: func() *Config {
					cfg := NewConfig()
					from := o11ystanzaentry.Field{FieldInterface: o11ystanzaentry.NewBodyField("from")}
					cfg.ParseFrom = &from
					cfg.Preset = "http"
					return cfg
				}(),
			},
		},
	}.Run(t)
}
