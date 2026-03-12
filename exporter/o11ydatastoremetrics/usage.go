package o11ydatastoremetrics

import (
	"fmt"
	"github.com/hanzoai/otel-collector/usage"
	"github.com/google/uuid"
	"go.opencensus.io/metric/metricdata"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"strings"
)

const (
	HanzoO11ySentMetricPointsKey      = "singoz_sent_metric_points"
	HanzoO11ySentMetricPointsBytesKey = "singoz_sent_metric_points_bytes"
	HanzoO11yMetricPointsCount        = "o11y_metric_points_count"
	HanzoO11yMetricPointsBytes        = "o11y_metric_points_bytes"
)

var (
	// Measures for usage
	ExporterHanzoO11ySentMetricPoints = stats.Int64(
		HanzoO11ySentMetricPointsKey,
		"Number of o11y metric points successfully sent to destination.",
		stats.UnitDimensionless)
	ExporterHanzoO11ySentMetricPointsBytes = stats.Int64(
		HanzoO11ySentMetricPointsBytesKey,
		"Total size of o11y metric points successfully sent to destination.",
		stats.UnitDimensionless)

	// Views for usage
	MetricPointsCountView = &view.View{
		Name:        HanzoO11yMetricPointsCount,
		Measure:     ExporterHanzoO11ySentMetricPoints,
		Description: "The number of metric points exported to o11y",
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{usage.TagTenantKey, usage.TagExporterIdKey},
	}
	MetricPointsBytesView = &view.View{
		Name:        HanzoO11yMetricPointsBytes,
		Measure:     ExporterHanzoO11ySentMetricPointsBytes,
		Description: "The size of metric points exported to o11y",
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{usage.TagTenantKey, usage.TagExporterIdKey},
	}
)

func UsageExporter(metrics []*metricdata.Metric, id uuid.UUID) (map[string]usage.Usage, error) {
	data := map[string]usage.Usage{}
	for _, metric := range metrics {
		if !strings.Contains(metric.Descriptor.Name, HanzoO11yMetricPointsCount) && !strings.Contains(metric.Descriptor.Name, HanzoO11yMetricPointsBytes) {
			continue
		}
		exporterIndex := usage.GetIndexOfLabel(metric.Descriptor.LabelKeys, usage.ExporterIDKey)
		tenantIndex := usage.GetIndexOfLabel(metric.Descriptor.LabelKeys, usage.TenantKey)
		if exporterIndex == -1 || tenantIndex == -1 {
			return nil, fmt.Errorf("usage: failed to get index of labels")
		}
		if strings.Contains(metric.Descriptor.Name, HanzoO11yMetricPointsCount) {
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
		} else if strings.Contains(metric.Descriptor.Name, HanzoO11yMetricPointsBytes) {
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
