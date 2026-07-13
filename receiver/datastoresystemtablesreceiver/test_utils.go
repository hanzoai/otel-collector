package datastoresystemtablesreceiver

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/consumer"
	"go.uber.org/zap"
)

type mockDatastoreQuerrier struct {
	tsNow uint32

	nextScrapeResult []QueryLog
}

var _ datastoreQuerier = (*mockDatastoreQuerrier)(nil)

func (t *mockDatastoreQuerrier) scrapeQueryLog(
	ctx context.Context, minTs uint32, maxTs uint32,
) ([]QueryLog, error) {
	return t.nextScrapeResult, nil
}

func (t *mockDatastoreQuerrier) unixTsNow(ctx context.Context) (
	uint32, error,
) {
	return t.tsNow, nil
}

func newTestReceiver(
	ch datastoreQuerier,
	scrapeIntervalSeconds uint32,
	scrapeDelaySeconds uint32,
	nextConsumer consumer.Logs,
) (*systemTablesReceiver, error) {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"stdout"}
	logger, err := config.Build()
	if err != nil {
		return nil, err
	}

	return &systemTablesReceiver{
		scrapeIntervalSeconds: scrapeIntervalSeconds,
		scrapeDelaySeconds:    scrapeDelaySeconds,
		datastore:            ch,
		nextConsumer:          nextConsumer,
		logger:                logger,
	}, nil
}

func makeTestQueryLog(hostname string, eventTime time.Time, query string) QueryLog {
	return QueryLog{
		Hostname:              hostname,
		EventType:             "QueryFinish",
		EventTime:             eventTime,
		EventTimeMicroseconds: eventTime,
		Query:                 query,
	}
}
