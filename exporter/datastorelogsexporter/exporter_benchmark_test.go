package datastorelogsexporter

import (
	"context"
	"testing"
	"time"

	driver "github.com/hanzo-ds/go/lib/driver"
	"github.com/hanzoai/otel-collector/pkg/pdatagen/plogsgen"
	"github.com/google/uuid"
	"github.com/jellydator/ttlcache/v3"
	cmock "github.com/hanzo-ds/mock"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/otel/metric/noop"
	"go.uber.org/zap"

	"github.com/hanzoai/otel-collector/exporter/datastorelogsexporter/internal/metadata"
)

// setupBenchmarkExporter creates a new exporter with mock Datastore client for benchmarking
func setupBenchmarkExporter(b *testing.B, mock driver.Conn) *datastoreLogsExporter {
	// keys cache is used to avoid duplicate inserts for the same attribute key.
	keysCache := ttlcache.New(
		ttlcache.WithTTL[string, struct{}](240*time.Minute),
		ttlcache.WithCapacity[string, struct{}](50000),
		ttlcache.WithDisableTouchOnHit[string, struct{}](),
	)
	go keysCache.Start()

	// resource fingerprint cache is used to avoid duplicate inserts for the same resource fingerprint.
	// the ttl is set to the same as the bucket rounded value i.e 1800 seconds.
	// if a resource fingerprint is seen in the bucket already, skip inserting it again.
	rfCache := ttlcache.New(
		ttlcache.WithTTL[string, struct{}](distributedLogsResourceV2Seconds*time.Second),
		ttlcache.WithDisableTouchOnHit[string, struct{}](),
		ttlcache.WithCapacity[string, struct{}](100000),
	)
	go rfCache.Start()

	opts := []LogExporterOption{
		WithLogger(zap.NewNop()),
		WithMeter(noop.NewMeterProvider().Meter(metadata.ScopeName)),
		WithKeysCache(keysCache),
		WithRFCache(rfCache),
		WithDatastoreClient(mock),
		WithConcurrency(4),
	}
	id := uuid.New()
	opts = append(opts, WithNewUsageCollector(id, mock))

	exporter, err := newExporter(
		exporter.Settings{},
		&Config{
			DSN: "datastore://localhost:9000/test",
			AttributesLimits: AttributesLimits{
				FetchKeysInterval: 2 * time.Second,
				MaxDistinctValues: 25000,
			},
		},
		opts...,
	)

	if err != nil {
		b.Fatalf("failed to create exporter: %v", err)
	}

	return exporter
}

func BenchmarkPushLogs_100k(b *testing.B) {
	ctx := context.Background()
	mock, err := cmock.NewClickHouseNative(nil)
	if err != nil {
		b.Fatalf("failed to create mock Datastore: %v", err)
	}

	exporter := setupBenchmarkExporter(b, mock)
	logs := plogsgen.Generate(plogsgen.WithLogRecordCount(100000))

	// Clean up caches and exporter after benchmark completes
	b.Cleanup(func() {
		if exporter != nil {
			_ = exporter.Shutdown(ctx)
		}
	})

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = exporter.pushLogsData(ctx, logs)
	}
}
