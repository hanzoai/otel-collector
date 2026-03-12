// Brought in as is from opentelemetry-collector-contrib
package trace

import (
	"path/filepath"
	"testing"

	o11ystanzaentry "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/entry"
	o11ystanzahelper "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/helper"
	"github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/operatortest"
)

func TestConfig(t *testing.T) {
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
				Name: "spanid",
				Expect: func() *Config {
					parseFrom := o11ystanzaentry.Field{FieldInterface: o11ystanzaentry.NewBodyField("app_span_id")}
					cfg := o11ystanzahelper.SpanIDConfig{}
					cfg.ParseFrom = &parseFrom

					c := NewConfig()
					c.SpanID = &cfg
					return c
				}(),
			},
			{
				Name: "traceid",
				Expect: func() *Config {
					parseFrom := o11ystanzaentry.Field{FieldInterface: o11ystanzaentry.NewBodyField("app_trace_id")}
					cfg := o11ystanzahelper.TraceIDConfig{}
					cfg.ParseFrom = &parseFrom

					c := NewConfig()
					c.TraceID = &cfg
					return c
				}(),
			},
			{
				Name: "trace_flags",
				Expect: func() *Config {
					parseFrom := o11ystanzaentry.Field{FieldInterface: o11ystanzaentry.NewBodyField("app_trace_flags_id")}
					cfg := o11ystanzahelper.TraceFlagsConfig{}
					cfg.ParseFrom = &parseFrom

					c := NewConfig()
					c.TraceFlags = &cfg
					return c
				}(),
			},
		},
	}.Run(t)
}
