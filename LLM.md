# otel-collector

# Hanzo O11y Otel Collector

Hanzo OpenTelemetry Collector distro. OTLP in → **ZAP** out → hanzoai/datastore
(ClickHouse). Adds `receiver/zapreceiver` + `exporter/zapexporter` over upstream.

## The wire is a luxfi/zap envelope — NOT `zap-proto/http`

This line used to say `zap-proto/http`, and that was wrong. The o11y receivers
decode a **JSON batch inside a luxfi/zap envelope**, dispatched on a MsgType in
the upper 8 bits of the flags field (spans=1, metrics=2, logs=3). Those are
different protocols, so nothing arrived and nothing errored — the receiver's zap
node simply never saw a type it recognised and the handler never ran. Two
`zap-proto` version bumps were spent chasing it before the docs were suspected.
It is corrected here because the doc WAS the bug the second time around.

## One ZAP identity per sender — do not derive it from config

`luxzap` keys its connection table by peer NodeID and admits exactly ONE
connection per identity; a duplicate is refused by closing before the handshake
reply, which the dialer reports as a bare `EOF`.

So a NodeID derived from anything that is the same in every copy of a config —
the exporter's component ID, most obviously — silently caps the whole fleet at
one connected sender. Measured 2026-08-01: 25 otel-agent pods all claiming
`otel-agent-zap`, one connected, 24 refused, ~88% of the fleet's logs discarded
while every pod reported Healthy and the errors read like a network fault.

`zapexporter.nodeID()` now derives it from **hostname + component ID** and there
is deliberately no config knob, since a knob is just a way to set one value on
every node. If you add another ZAP sender anywhere, give it a per-process
identity or it will take the fleet's connection from the agents.

## Span kind goes on the wire LOWERCASE

pdata renders SpanKind title-cased (`"Server"`); the receiver matches `server`
or `SPAN_KIND_SERVER` and defaults everything else to `internal`. Sent
title-cased, every span of every kind stored as internal — no error, trace still
renders, only the entry/exit distinction quietly gone. `translateSpan` lowercases;
`TestSpanKindIsTheReceiversVocabulary` keeps it that way.

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
