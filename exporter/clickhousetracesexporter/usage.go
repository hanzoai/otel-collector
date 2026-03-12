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
	Hanzo O11ySentSpansKey      = "singoz_sent_spans"
	Hanzo O11ySentSpansBytesKey = "singoz_sent_spans_bytes"
	Hanzo O11ySpansCount        = "o11y_spans_count"
	Hanzo O11ySpansBytes        = "o11y_spans_bytes"
)

var (
	// Measures for usage
	ExporterHanzo O11ySentSpans = stats.Int64(
		Hanzo O11ySentSpansKey,
		"Number of o11y log records successfully sent to destination.",
		stats.UnitDimensionless)
	ExporterHanzo O11ySentSpansBytes = stats.Int64(
		Hanzo O11ySentSpansBytesKey,
		"Total size of o11y log records successfully sent to destination.",
		stats.UnitDimensionless)

	// Views for usage
	SpansCountView = &view.View{
		Name:        Hanzo O11ySpansCount,
		Measure:     ExporterHanzo O11ySentSpans,
		Description: "The number of spans exported to o11y",
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{usage.TagTenantKey, usage.TagExporterIdKey},
	}
	SpansCountBytesView = &view.View{
		Name:        Hanzo O11ySpansBytes,
		Measure:     ExporterHanzo O11ySentSpansBytes,
		Description: "The size of spans exported to o11y",
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{usage.TagTenantKey, usage.TagExporterIdKey},
	}
)

func UsageExporter(metrics []*metricdata.Metric, id uuid.UUID) (map[string]usage.Usage, error) {
	data := map[string]usage.Usage{}
	for _, metric := range metrics {
		if !strings.Contains(metric.Descriptor.Name, Hanzo O11ySpansCount) && !strings.Contains(metric.Descriptor.Name, Hanzo O11ySpansBytes) {
			continue
		}
		exporterIndex := usage.GetIndexOfLabel(metric.Descriptor.LabelKeys, usage.ExporterIDKey)
		tenantIndex := usage.GetIndexOfLabel(metric.Descriptor.LabelKeys, usage.TenantKey)
		if exporterIndex == -1 || tenantIndex == -1 {
			return nil, fmt.Errorf("usage: failed to get index of labels")
		}
		if strings.Contains(metric.Descriptor.Name, Hanzo O11ySpansCount) {
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
		} else if strings.Contains(metric.Descriptor.Name, Hanzo O11ySpansBytes) {
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
