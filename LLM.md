# otel-collector

# Hanzo O11y Otel Collector

Hanzo OpenTelemetry Collector distro. ZAP-native (`zap-proto/http` :4319) +
OTLP (:4317/:4318) in → hanzoai/datastore (ClickHouse) out. Adds
`receiver/zapreceiver` + `exporter/zapexporter` over upstream.

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
