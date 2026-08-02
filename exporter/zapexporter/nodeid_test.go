package zapexporter

import (
	"context"
	"testing"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/plog"
)

// fleetSettings reproduces the production situation these tests exist for: N
// collectors running ONE ConfigMap, so every one of them names its exporter
// `zap` and carries the IDENTICAL component ID.
//
// exportertest.NewNopSettings must not be used here — it stamps a fresh UUID as
// the component name, so two exporters built from it differ even under the
// pre-fix code. A test built on it passes against the bug, which is the exact
// failure mode this package's own test file warns about: "a green test against a
// server nobody runs is worse than no test".
func fleetSettings() exporter.Settings {
	set := exportertest.NewNopSettings(typeStr)
	set.ID = component.NewID(typeStr)
	return set
}

// startOn brings up a logs exporter as if it were running on host `host`,
// pointed at addr. The hostname stub is only in effect across Start, which is
// where the ZAP node (and therefore its identity) is created.
func startOn(t *testing.T, host, addr string) exporter.Logs {
	t.Helper()
	orig := hostname
	hostname = func() (string, error) { return host, nil }
	defer func() { hostname = orig }()

	cfg := NewFactory().CreateDefaultConfig().(*Config)
	cfg.Endpoint = addr
	exp, err := NewFactory().CreateLogs(context.Background(), fleetSettings(), cfg)
	if err != nil {
		t.Fatalf("create (%s): %v", host, err)
	}
	if err := exp.Start(context.Background(), componenttest.NewNopHost()); err != nil {
		t.Fatalf("start (%s): %v", host, err)
	}
	t.Cleanup(func() { _ = exp.Shutdown(context.Background()) })
	return exp
}

func oneLog(service, body string) plog.Logs {
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", service)
	rl.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().Body().SetStr(body)
	return ld
}

// TestTwoAgentsOneReceiver is the regression test for the fleet-wide telemetry
// loss measured 2026-08-01.
//
// luxfi/zap admits ONE connection per peer NodeID and refuses duplicates by
// closing without a handshake reply, so the loser reads EOF. When the NodeID was
// derived from the component ID alone, all 25 otel-agent pods claimed
// "otel-agent-zap": one was connected and 24 were refused forever, dropping ~88%
// of the fleet's logs while reporting a network error.
//
// Two senders that differ ONLY by host, one receiver, both batches must land.
// Against the pre-fix exporter the second sender fails Start's dial and its
// export returns `connect ...: EOF`, so this test fails — which is the bug.
func TestTwoAgentsOneReceiver(t *testing.T) {
	addr, got := receiver(t)

	a := startOn(t, "otel-agent-node-a", addr)
	b := startOn(t, "otel-agent-node-b", addr)

	if err := a.ConsumeLogs(context.Background(), oneLog("cart", "from node-a")); err != nil {
		t.Fatalf("node-a export: %v", err)
	}
	if err := b.ConsumeLogs(context.Background(), oneLog("cart", "from node-b")); err != nil {
		// This is the exact production failure: the second agent is refused.
		t.Fatalf("node-b export: %v  (a second agent could not deliver — NodeID collision)", err)
	}

	eventually(t, func() bool { n, _, _ := got.counts(); return n == 2 },
		"receiver did not get BOTH agents' batches — one sender was locked out by a duplicate NodeID")

	got.mu.Lock()
	defer got.mu.Unlock()
	bodies := map[string]bool{}
	for _, batch := range got.logs {
		for _, r := range batch.Records {
			bodies[r.Body] = true
		}
	}
	if !bodies["from node-a"] || !bodies["from node-b"] {
		t.Fatalf("both agents must be represented, got %v", bodies)
	}
}

// TestNodeIDIsUniquePerHostAndComponent states the two axes the identity must
// separate: the same config on different hosts, and two exporters inside one
// collector. Both collided before the fix.
func TestNodeIDIsUniquePerHostAndComponent(t *testing.T) {
	orig := hostname
	defer func() { hostname = orig }()

	idOn := func(host string) string {
		hostname = func() (string, error) { return host, nil }
		e := newExporter(fleetSettings(), &Config{})
		return e.nodeID()
	}

	if a, b := idOn("node-a"), idOn("node-b"); a == b {
		t.Fatalf("same identity on two hosts: %q — every agent would be refused but one", a)
	}
	if id := idOn("node-a"); id != "otel-agent-node-a-zap" {
		t.Fatalf("identity should name the host and the component, got %q", id)
	}

	// A blank hostname must not fall back to a shared constant.
	hostname = func() (string, error) { return "", nil }
	e := newExporter(fleetSettings(), &Config{})
	if x, y := e.nodeID(), e.nodeID(); x == y {
		t.Fatalf("blank hostname produced a reusable identity %q — collisions return", x)
	}
}
