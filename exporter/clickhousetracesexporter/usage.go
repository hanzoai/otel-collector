package clickhousetracesexporter

import (
	"fmt"
	"strings"

	"github.com/hanzoai/otel-collector/usage"
	"github.com/google/uuid"
	"go.opencensus.io/metric/metricdata"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

const (
	HanzoO11ySentSpansKey      = "singoz_sent_spans"
	HanzoO11ySentSpansBytesKey = "singoz_sent_spans_bytes"
	HanzoO11ySpansCount        = "o11y_spans_count"
	HanzoO11ySpansBytes        = "o11y_spans_bytes"
)

var (
	// Measures for usage
	ExporterHanzoO11ySentSpans = stats.Int64(
		HanzoO11ySentSpansKey,
		"Number of o11y log records successfully sent to destination.",
		stats.UnitDimensionless)
	ExporterHanzoO11ySentSpansBytes = stats.Int64(
		HanzoO11ySentSpansBytesKey,
		"Total size of o11y log records successfully sent to destination.",
		stats.UnitDimensionless)

	// Views for usage
	SpansCountView = &view.View{
		Name:        HanzoO11ySpansCount,
		Measure:     ExporterHanzoO11ySentSpans,
		Description: "The number of spans exported to o11y",
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{usage.TagTenantKey, usage.TagExporterIdKey},
	}
	SpansCountBytesView = &view.View{
		Name:        HanzoO11ySpansBytes,
		Measure:     ExporterHanzoO11ySentSpansBytes,
		Description: "The size of spans exported to o11y",
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{usage.TagTenantKey, usage.TagExporterIdKey},
	}
)

func UsageExporter(metrics []*metricdata.Metric, id uuid.UUID) (map[string]usage.Usage, error) {
	data := map[string]usage.Usage{}
	for _, metric := range metrics {
		if !strings.Contains(metric.Descriptor.Name, HanzoO11ySpansCount) && !strings.Contains(metric.Descriptor.Name, HanzoO11ySpansBytes) {
			continue
		}
		exporterIndex := usage.GetIndexOfLabel(metric.Descriptor.LabelKeys, usage.ExporterIDKey)
		tenantIndex := usage.GetIndexOfLabel(metric.Descriptor.LabelKeys, usage.TenantKey)
		if exporterIndex == -1 || tenantIndex == -1 {
			return nil, fmt.Errorf("usage: failed to get index of labels")
		}
		if strings.Contains(metric.Descriptor.Name, HanzoO11ySpansCount) {
			for _, v := range metric.TimeSeries {
				if v.LabelValues[exporterIndex].Value != id.String() {
					continue
				}
				tenant := v.LabelValues[tenantIndex].Value
				if d, ok := data[tenant]; ok {
					d.Count = v.Points[0].Value.(int64)
					data[tenant] = d
				} else {
					data[tenant] = usage.Usage{
						Count: v.Points[0].Value.(int64),
					}
				}
			}
		} else if strings.Contains(metric.Descriptor.Name, HanzoO11ySpansBytes) {
			for _, v := range metric.TimeSeries {
				if v.LabelValues[exporterIndex].Value != id.String() {
					continue
				}
				tenant := v.LabelValues[tenantIndex].Value
				if d, ok := data[tenant]; ok {
					d.Size = v.Points[0].Value.(int64)
					data[tenant] = d
				} else {
					data[tenant] = usage.Usage{
						Size: v.Points[0].Value.(int64),
					}
				}
			}
		}
	}
	return data, nil
}
