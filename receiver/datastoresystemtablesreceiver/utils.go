package datastoresystemtablesreceiver

import (
	"context"

	"github.com/hanzo-ds/go"
)

func newDatastoreClient(ctx context.Context, dsn string) (datastore.Conn, error) {
	options, err := datastore.ParseDSN(dsn)
	if err != nil {
		return nil, err
	}

	db, err := datastore.Open(options)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(ctx); err != nil {
		return nil, err
	}

	return db, nil
}
