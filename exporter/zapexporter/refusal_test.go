package zapexporter

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/plog"
)

// refusingPeer accepts the connection and closes it immediately, without a
// handshake reply. That is precisely what luxfi/zap does to a duplicate peer
// ID — handleConn sees an ID it is already holding and returns without
// answering, so the dialling side reads EOF mid-handshake.
//
// The distinction from deafPeer matters: a deaf peer makes the caller wait for
// its deadline, and a refusing peer fails it instantly. Only the first was ever
// counted, and the second is the one with no way out — redialling presents the
// same refused identity forever.
func refusingPeer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().String()
}

// The headline: a refused connect must reach the stall budget and rebuild the
// node, because a rebuild is what mints the new identity the receiver will
// accept. Before this, the budget read only context.DeadlineExceeded, so an
// agent refused at handshake looped on EOF forever while its queue filled to
// the ceiling and began discarding.
func TestARefusedConnectRebuildsTheNode(t *testing.T) {
	cfg := NewFactory().CreateDefaultConfig().(*Config)
	cfg.Endpoint = refusingPeer(t)

	e := newExporter(exportertest.NewNopSettings(typeStr), cfg)
	if err := e.Start(context.Background(), componenttest.NewNopHost()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = e.Shutdown(context.Background()) }()

	e.mu.Lock()
	before := e.node
	e.mu.Unlock()

	ld := plog.NewLogs()
	ld.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty()

	// Start's own dial may already have spent part of the budget; drive enough
	// attempts that a budget which counts refusals must trip.
	for i := 0; i < stallsBeforeReset+2; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = e.pushLogs(ctx, ld)
		cancel()
	}

	e.mu.Lock()
	after := e.node
	e.mu.Unlock()

	if after == before {
		t.Fatal("a refused connect never rebuilt the node: the agent would loop on EOF forever")
	}
	if e.lastReset.Load() == 0 {
		t.Fatal("rebuild did not record its time, so the floor cannot hold")
	}
}

// The floor. A receiver that is merely down refuses every dial, and an unfloored
// budget would mint identities several times a second on every agent at once.
func TestRebuildsAreFloored(t *testing.T) {
	cfg := NewFactory().CreateDefaultConfig().(*Config)
	cfg.Endpoint = refusingPeer(t)

	e := newExporter(exportertest.NewNopSettings(typeStr), cfg)
	if err := e.Start(context.Background(), componenttest.NewNopHost()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = e.Shutdown(context.Background()) }()

	e.resetNode()
	first := e.lastReset.Load()
	if first == 0 {
		t.Fatal("first rebuild should have been allowed")
	}
	e.mu.Lock()
	node := e.node
	e.mu.Unlock()

	for i := 0; i < 50; i++ {
		e.resetNode()
	}

	if got := e.lastReset.Load(); got != first {
		t.Fatalf("the floor did not hold: rebuild time moved from %d to %d", first, got)
	}
	e.mu.Lock()
	same := e.node == node
	e.mu.Unlock()
	if !same {
		t.Fatal("the floor did not hold: the node was rebuilt again inside the interval")
	}
}

// The two "ask again" failures must NOT spend the budget. Ordinary concurrency
// between the three signal pipelines produces errDialInFlight constantly, and
// counting it would let a healthy exporter rebuild itself.
func TestAskAgainFailuresDoNotCountAsStalls(t *testing.T) {
	if stalled(errDialInFlight) {
		t.Error("a dial already in flight is concurrency, not a stall")
	}
	if stalled(errNoNode) {
		t.Error("a node not yet started is not a stall")
	}
	if stalled(nil) {
		t.Error("success is not a stall")
	}
	if !stalled(context.DeadlineExceeded) {
		t.Error("a deadline-abandoned attempt must still count, as it always did")
	}
	if !stalled(errors.New("EOF")) {
		t.Error("a refused connect must count: it is the failure with no way out")
	}
	// Wrapped, which is how send() actually returns them.
	if stalled(errors.Join(errDialInFlight, errors.New("context"))) {
		t.Error("a wrapped ask-again sentinel must still be recognised")
	}
}
