# Datastore System Tables Receiver

Connects to datastore to collect monitoring data from datastore system tables

Only collects logs from query_log table right now.
Support for other system tables like query_views_log, query_thread_log etc may be added later as needed.


## Example Configuration

```yaml
receivers:
  datastoresystemtablesreceiver:
    # required
    dsn: tcp://datastore:9000/

    # optional. Should be set to name of the cluster when scraping query logs from a clustered Datastore deployment
    cluster_name: "cluster-name"

    query_log_scrape_config:
      scrape_interval_seconds: 20
      # required. min_scrape_delay_seconds must be set to a duration larger
      # than flush_interval_milliseconds settings for query_log
      # For details, see https://clickhouse.com/docs/en/operations/server-configuration-parameters/settings#query-log
      min_scrape_delay_seconds: 8
```
