// Package zapexporter ships OpenTelemetry signals (traces, logs, metrics) to a
// collector's ZAP-native OTLP receiver (receiver/zapreceiver) over Hanzo's ZAP
// transport (github.com/zap-proto/http) — never OTLP-over-HTTP (:4318) or
// OTLP-over-gRPC (:4317).
//
// The payload is standard OTLP protobuf; only the wire is ZAP. This is the
// exporter the filelog log-agent DaemonSet uses to forward node pod/container
// logs to the central collector over ZAP.
package zapexporter
