// Package components is the collector's component registry.
//
// It carries the components Hanzo pipelines actually name, and nothing else.
// Upstream's contrib distribution registers a few hundred receivers, exporters
// and processors covering every vendor in the ecosystem; each one compiles into
// the binary and brings its own dependency tree whether a pipeline references
// it or not.
//
// The transport is ZAP (zap-proto/http, :4319). OTLP stays as a receiver so
// third-party OpenTelemetry SDKs can still ship to us, and it is the last thing
// in this build that links gRPC — retiring it is a matter of moving the
// remaining OTLP senders onto the ZAP wire, not of editing this file.
package components

import (
	"github.com/hanzoai/otel-collector/exporter/datastorelogsexporter"
	"github.com/hanzoai/otel-collector/exporter/datastoretracesexporter"
	"github.com/hanzoai/otel-collector/exporter/o11ydatastoremetrics"
	"github.com/hanzoai/otel-collector/exporter/zapexporter"
	o11yhealthcheckextension "github.com/hanzoai/otel-collector/extension/healthcheckextension"
	"github.com/hanzoai/otel-collector/processor/o11yspanmetricsprocessor"
	"github.com/hanzoai/otel-collector/receiver/zapreceiver"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusexporter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/pprofextension"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/filterprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/k8sattributesprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourcedetectionprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourceprocessor"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/filelogreceiver"
	"go.opentelemetry.io/collector/exporter/debugexporter"
	"go.opentelemetry.io/collector/exporter/nopexporter"
	"go.opentelemetry.io/collector/extension/zpagesextension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/batchprocessor"
	"go.opentelemetry.io/collector/processor/memorylimiterprocessor"
	"go.opentelemetry.io/collector/receiver/nopreceiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/service/telemetry/otelconftelemetry"
	"go.uber.org/multierr"
)

// Components returns the factories the collector is built from.
//
// A component belongs here when a Hanzo pipeline names it. One that nothing
// configures costs binary size and dependency surface for no reach.
func Components() (otelcol.Factories, error) {
	var errs []error

	extensions, err := otelcol.MakeFactoryMap(
		// "o11y_health_check" — the fork's, and the only one. Upstream's
		// "health_check" is a second way to do the same thing, and its release
		// train does not line up with the contrib version the rest of this
		// build pins.
		o11yhealthcheckextension.NewFactory(),
		pprofextension.NewFactory(),
		zpagesextension.NewFactory(),
	)
	errs = append(errs, err)

	receivers, err := otelcol.MakeFactoryMap(
		// The canonical transport — Hanzo services ship over the ZAP wire.
		zapreceiver.NewFactory(),
		// Interop for third-party OpenTelemetry SDKs.
		otlpreceiver.NewFactory(),
		filelogreceiver.NewFactory(),
		nopreceiver.NewFactory(),
	)
	errs = append(errs, err)

	processors, err := otelcol.MakeFactoryMap[processor.Factory](
		batchprocessor.NewFactory(),
		memorylimiterprocessor.NewFactory(),
		filterprocessor.NewFactory(),
		k8sattributesprocessor.NewFactory(),
		resourcedetectionprocessor.NewFactory(),
		resourceprocessor.NewFactory(),
		o11yspanmetricsprocessor.NewFactory(),
	)
	errs = append(errs, err)

	exporters, err := otelcol.MakeFactoryMap(
		datastoretracesexporter.NewFactory(),
		datastorelogsexporter.NewFactory(),
		o11ydatastoremetrics.NewFactory(),
		prometheusexporter.NewFactory(),
		// Forwards to another collector's ZAP receiver; the log agent uses it.
		zapexporter.NewFactory(),
		debugexporter.NewFactory(),
		nopexporter.NewFactory(),
	)
	errs = append(errs, err)

	factories := otelcol.Factories{
		Extensions: extensions,
		Receivers:  receivers,
		Processors: processors,
		Exporters:  exporters,
		Telemetry:  otelconftelemetry.NewFactory(),
	}

	return factories, multierr.Combine(errs...)
}
