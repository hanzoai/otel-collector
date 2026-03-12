// Brought in as is from logstransform processor in opentelemetry-collector-contrib
package o11ylogspipelineprocessor

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap/confmaptest"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/entry"

	o11ylogspipelinestanzaadapter "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/adapter"
	o11ystanzaentry "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/entry"
	o11ylogspipelinestanzaoperator "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator"
	o11ystanzahelper "github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/helper"
	"github.com/hanzoai/otel-collector/processor/o11ylogspipelineprocessor/stanza/operator/operators/regex"
)

func TestLoadConfig(t *testing.T) {
	cm, err := confmaptest.LoadConf(filepath.Join("testdata", "config.yaml"))
	require.NoError(t, err)
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	assert.NoError(t, cm.Unmarshal(cfg))
	assert.Equal(t, &Config{
		BaseConfig: o11ylogspipelinestanzaadapter.BaseConfig{
			Operators: []o11ylogspipelinestanzaoperator.Config{
				{
					Builder: func() *regex.Config {
						cfg := regex.NewConfig()
						cfg.Regex = "^(?P<time>\\d{4}-\\d{2}-\\d{2} \\d{2}:\\d{2}:\\d{2}) (?P<sev>[A-Z]*) (?P<msg>.*)$"
						sevField := o11ystanzaentry.Field{FieldInterface: entry.NewAttributeField("sev")}
						sevCfg := o11ystanzahelper.NewSeverityConfig()
						sevCfg.ParseFrom = &sevField
						cfg.SeverityConfig = &sevCfg
						timeField := o11ystanzaentry.Field{FieldInterface: entry.NewAttributeField("time")}
						timeCfg := o11ystanzahelper.NewTimeParser()
						timeCfg.Layout = "%Y-%m-%d %H:%M:%S"
						timeCfg.ParseFrom = &timeField
						cfg.TimeParser = &timeCfg
						return cfg
					}(),
				},
			},
		},
	}, cfg)
}
