# otel-collector

# Hanzo O11y Otel Collector

Hanzo OpenTelemetry Collector distro → hanzoai/datastore out. Adds
`receiver/zapreceiver` + `exporter/zapexporter` over upstream.

## The OTLZ wire is a luxfi/zap envelope — NOT zap-proto/http

`exporter/zapexporter` sends a JSON batch inside a **luxfi/zap** envelope, tagged
by MsgType in the **upper 8 bits of the flags field** (`FinishWithFlags(t<<8)`):
`MsgSpanBatch=1`, `MsgMetricBatch=2`, `MsgLogBatch=3`. That is what o11y's
`pkg/zap{,log,metric}receiver` dispatch on, and what `luxfi/trace` emits. The
receiver replies with NOTHING, so the sender uses `Send`, never `Call`.

This line used to say `zap-proto/http :4319`, and that error cost two days of
telemetry: the exporter marshalled OTLP protobuf and POSTed it to `/v1/logs` over
`zap-proto/http`, so the collector's zap.Node never saw an envelope type it knew,
its handler was never invoked, nothing was written back, and the agent sat in
`read response` until timeout. `o11y_logs.logs_v2` took 0 rows while the agent
retried on backoff. Two `zap-proto` version bumps chased the wrong library —
the receiver does not speak that protocol at any version.

Ports: the collector binds traces on **:4317** and logs on **:4318** (separate,
because each receiver opens its own listener and two on one address is
`address already in use`, which killed the whole pipeline).

## CI = root `hanzo.yml` + `.github/workflows/cicd.yml` importing `hanzoai/ci`

- **`hanzo.yml`** (root, NO leading dot) declares `images:` / `test:` / `deploy:`
  / `kms:`. **`.github/workflows/cicd.yml`** is ~7 lines:
  `uses: hanzoai/ci/.github/workflows/build.yml@main` + `secrets: inherit`.
- Runs on our self-hosted **arc** pool (`hanzo-build-linux-amd64`) — **no
  GitHub-hosted minutes**. GHCR push = automatic workflow token; KMS =
  deploy `KUBECONFIG` only.
- Two self-building images (multi-stage, compile in-image, no CI pre-build):
  - `collector` → `cmd/o11yotelcollector/Dockerfile.selfbuild` →
    `ghcr.io/hanzoai/otel-collector:zap-native-latest` +
    `:sha-<short>-amd64-zap-native`.
  - `schema-migrator` → `cmd/o11yschemamigrator/Dockerfile.selfbuild` →
    `ghcr.io/hanzoai/o11y-schema-migrator:latest` + `:sha-<short>-amd64`.
- The `zap-native` tag = the collector build that includes zapreceiver +
  zapexporter. Deploy manifests (config ConfigMap with the `zap` receiver on
  :4319 + the `otel-agent` log DaemonSet that ships over ZAP) live in
  hanzoai/universe `infra/k8s/monitoring` + `infra/k8s/o11y`.

**This is the going-forward pattern for ALL native-PaaS repos.** Onboard = add
`hanzo.yml` + `cicd.yml`; nothing else. The legacy `hanzoai/.github`
`docker-build.yml` and `o11y/primus.workflows` reusables were removed (broken:
runtime-validation `jobs: 0`, and primus is inaccessible).
