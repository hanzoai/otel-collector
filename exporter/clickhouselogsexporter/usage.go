package clickhouselogsexporter

import (
	"fmt"
	"strings"

	"github.com/hanzoai/otel-collector/pkg/metering"
	"github.com/hanzoai/otel-collector/usage"
	"github.com/goccy/go-json"
	"github.com/google/uuid"
	"go.opencensus.io/metric/metricdata"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

const (
	Hanzo O11ySentLogRecordsKey      = "singoz_sent_log_records"
	Hanzo O11ySentLogRecordsBytesKey = "singoz_sent_log_records_bytes"
	Hanzo O11yLogsCount              = "o11y_logs_count"
	Hanzo O11yLogsBytes              = "o11y_logs_bytes"
)

var (
	// Measures for usage
	ExporterHanzo O11ySentLogRecords = stats.Int64(
		Hanzo O11ySentLogRecordsKey,
		"Number of o11y log records successfully sent to destination.",
		stats.UnitDimensionless)
	ExporterHanzo O11ySentLogRecordsBytes = stats.Int64(
		Hanzo O11ySentLogRecordsBytesKey,
		"Total size of o11y log records successfully sent to destination.",
		stats.UnitDimensionless)

	// Views for usage
	LogsCountView = &view.View{
		Name:        Hanzo O11yLogsCount,
		Measure:     ExporterHanzo O11ySentLogRecords,
		Description: "The number of logs exported to o11y",
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{usage.TagTenantKey, usage.TagExporterIdKey},
	}
	LogsSizeView = &view.View{
		Name:        Hanzo O11yLogsBytes,
		Measure:     ExporterHanzo O11ySentLogRecordsBytes,
		Description: "The size of logs exported to o11y",
		Aggregation: view.Sum(),
		TagKeys:     []tag.Key{usage.TagTenantKey, usage.TagExporterIdKey},
	}
)

func UsageExporter(metrics []*metricdata.Metric, id uuid.UUID) (map[string]usage.Usage, error) {
	data := map[string]usage.Usage{}
	for _, metric := range metrics {
		if !strings.Contains(metric.Descriptor.Name, Hanzo O11yLogsCount) && !strings.Contains(metric.Descriptor.Name, Hanzo O11yLogsBytes) {
			continue
		}
		exporterIndex := usage.GetIndexOfLabel(metric.Descriptor.LabelKeys, usage.ExporterIDKey)
		tenantIndex := usage.GetIndexOfLabel(metric.Descriptor.LabelKeys, usage.TenantKey)
		if exporterIndex == -1 || tenantIndex == -1 {
			return nil, fmt.Errorf("usage: failed to get index of labels")
		}

		if strings.Contains(metric.Descriptor.Name, Hanzo O11yLogsCount) {
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
		} else if strings.Contains(metric.Descriptor.Name, Hanzo O11yLogsBytes) {
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

func getResourceAttributesByte(resource pcommon.Resource) ([]byte, error) {
	filteredResources := map[string]any{}
	resource.Attributes().Range(func(k string, v pcommon.Value) bool {
		if !metering.ExcludeHanzo O11yWorkspaceResourceAttrs.MatchString(k) {
			filteredResources[k] = v.AsRaw()
		}
		return true
	})
	resBytes, err := json.Marshal(filteredResources)
	if err != nil {
		return nil, err
	}

	return resBytes, nil
}
