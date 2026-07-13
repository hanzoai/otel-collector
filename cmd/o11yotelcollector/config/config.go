package config

import (
	"time"

	o11ycolFeatureGate "github.com/hanzoai/otel-collector/featuregate"
	"github.com/spf13/cobra"
	otelcolFeatureGate "go.opentelemetry.io/collector/featuregate"
)

var (
	Collector         collector
	Datastore        datastore
	MigrateReady      migrateReady
	MigrateBootstrap  migrateBootstrap
	MigrateSyncCheck  migrateSyncCheck
	MigrateSyncUp     migrateSyncUp
	MigrateAsyncCheck migrateAsyncCheck
	MigrateAsyncUp    migrateAsyncUp
)

type collector struct {
	Config        string
	ManagerConfig string
	CopyPath      string
}

func (cfg *collector) RegisterFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&cfg.Config, "config", "", "File path for the collector configuration")
	cmd.PersistentFlags().StringVar(&cfg.ManagerConfig, "manager-config", "", "File path for the agent manager configuration")
	cmd.PersistentFlags().StringVar(&cfg.CopyPath, "copy-path", "/etc/otel/o11ycol-config.yaml", "File path for the copied collector configuration")
	cmd.PersistentFlags().Var(o11ycolFeatureGate.NewFlag(otelcolFeatureGate.GlobalRegistry()), "feature-gates",
		"Comma-delimited list of feature gate identifiers. Prefix with '-' to disable the feature. '+' or no prefix will enable the feature.")
}

type datastore struct {
	DSN         string
	Cluster     string
	Replication bool
}

func (cfg *datastore) RegisterFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&cfg.DSN, "datastore-dsn", "tcp://0.0.0.0:9001", "DSN for datastore connection")
	cmd.PersistentFlags().StringVar(&cfg.Cluster, "datastore-cluster", "cluster", "Name of the datastore cluster to connect")
	cmd.PersistentFlags().BoolVar(&cfg.Replication, "datastore-replication", true, "Set true if replication is enabled in the datastore cluster")
}

type migrateReady struct {
	Timeout time.Duration
}

func (cfg *migrateReady) RegisterFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().DurationVar(&cfg.Timeout, "timeout", time.Duration(10*time.Second), "Timeout for migrate ready operation")
}

type migrateSyncCheck struct {
	Timeout time.Duration
}

func (cfg *migrateSyncCheck) RegisterFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().DurationVar(&cfg.Timeout, "timeout", time.Duration(10*time.Second), "Timeout for sync check operation")
}

type migrateBootstrap struct {
	Timeout time.Duration
}

func (cfg *migrateBootstrap) RegisterFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().DurationVar(&cfg.Timeout, "timeout", time.Duration(15*time.Minute), "Timeout for bootstrap operation")
}

type migrateSyncUp struct {
	Timeout time.Duration
}

func (cfg *migrateSyncUp) RegisterFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().DurationVar(&cfg.Timeout, "timeout", time.Duration(10*time.Second), "Timeout for sync up operation")
}

type migrateAsyncCheck struct {
	Timeout time.Duration
}

func (cfg *migrateAsyncCheck) RegisterFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().DurationVar(&cfg.Timeout, "timeout", time.Duration(10*time.Second), "Timeout for async check operation")
}

type migrateAsyncUp struct {
	Timeout time.Duration
}

func (cfg *migrateAsyncUp) RegisterFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().DurationVar(&cfg.Timeout, "timeout", time.Duration(10*time.Second), "Timeout for async up operation")
}
