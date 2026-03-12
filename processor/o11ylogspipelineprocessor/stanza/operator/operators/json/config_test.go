// Brought in as is from opentelemetry-collector-contrib
package json

import (
	"path/filepath"
	"testing"

	o11ystanzaentry "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/entry"
	o11ystanzahelper "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/helper"
	"github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/operatortest"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/entry"
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
				Name: "parse_from_simple",
				Expect: func() *Config {
					cfg := NewConfig()
					cfg.ParseFrom = o11ystanzaentry.Field{FieldInterface: o11ystanzaentry.NewBodyField("from")}
					return cfg
				}(),
			},
			{
				Name: "parse_to_simple",
				Expect: func() *Config {
					cfg := NewConfig()
					cfg.ParseTo = o11ystanzaentry.RootableField{Field: o11ystanzaentry.Field{FieldInterface: o11ystanzaentry.NewBodyField("log")}}
					return cfg
				}(),
			},
			{
				Name: "timestamp",
				Expect: func() *Config {
					cfg := NewConfig()
					parseField := o11ystanzaentry.Field{FieldInterface: o11ystanzaentry.NewBodyField("timestamp_field")}
					newTime := o11ystanzahelper.TimeParser{
						LayoutType: "strptime",
						Layout:     "%Y-%m-%d",
						ParseFrom:  &parseField,
					}
					cfg.TimeParser = &newTime
					return cfg
				}(),
			},
			{
				Name: "severity",
				Expect: func() *Config {
					cfg := NewConfig()
					parseField := o11ystanzaentry.Field{FieldInterface: o11ystanzaentry.NewBodyField("severity_field")}
					severityParser := o11ystanzahelper.NewSeverityConfig()
					severityParser.ParseFrom = &parseField
					mapping := map[string]any{
						"critical": "5xx",
						"error":    "4xx",
						"info":     "3xx",
						"debug":    "2xx",
					}
					severityParser.Mapping = mapping
					cfg.SeverityConfig = &severityParser
					return cfg
				}(),
			},
			{
				Name: "scope_name",
				Expect: func() *Config {
					cfg := NewConfig()
					loggerNameParser := o11ystanzahelper.NewScopeNameParser()
					loggerNameParser.ParseFrom = o11ystanzaentry.Field{FieldInterface: o11ystanzaentry.NewBodyField("logger_name_field")}
					cfg.ScopeNameParser = &loggerNameParser
					return cfg
				}(),
			},
			{
				Name: "parse_to_attributes",
				Expect: func() *Config {
					p := NewConfig()
					p.ParseTo = o11ystanzaentry.RootableField{Field: o11ystanzaentry.Field{FieldInterface: entry.NewAttributeField()}}
					return p
				}(),
			},
			{
				Name: "parse_to_body",
				Expect: func() *Config {
					p := NewConfig()
					p.ParseTo = o11ystanzaentry.RootableField{Field: o11ystanzaentry.Field{FieldInterface: o11ystanzaentry.NewBodyField()}}
					return p
				}(),
			},
			{
				Name: "parse_to_resource",
				Expect: func() *Config {
					p := NewConfig()
					p.ParseTo = o11ystanzaentry.RootableField{Field: o11ystanzaentry.Field{FieldInterface: entry.NewResourceField()}}
					return p
				}(),
			},
			{
				Name: "json_flattening",
				Expect: func() *Config {
					p := NewConfig()
					p.ParseTo = o11ystanzaentry.RootableField{Field: o11ystanzaentry.Field{FieldInterface: entry.NewAttributeField()}}
					p.EnableFlattening = true
					p.MaxFlatteningDepth = 4
					p.EnablePaths = true
					p.PathPrefix = "parsed"
					return p
				}(),
			},
		},
	}.Run(t)
}
