// Package zapreceiver ingests OpenTelemetry signals (traces, logs, metrics)
// carried over Hanzo's native ZAP transport (github.com/zap-proto/http) rather
// than OTLP-over-HTTP (:4318) or OTLP-over-gRPC (:4317).
//
// The wire is pure ZAP: each request/response is one ZAP frame over the
// zap-proto/http length-prefixed transport (X-Wing PQ-KEM handshake at the
// transport layer, per the zap-proto spec) — there is no net/http and no gRPC
// in the path. The PAYLOAD is standard OTLP protobuf/JSON (the OpenTelemetry
// data model), so full OTLP fidelity and pipeline interop are preserved while
// the transport is ZAP-native.
//
// This is the canonical transport Hanzo's own services (cloud, gateway,
// agents, the filelog log-agent) use to ship telemetry into the one
// hanzoai/datastore. Standard OTLP receivers remain available on the same
// collector as an interop endpoint for third-party OTel SDKs.
package zapreceiver
