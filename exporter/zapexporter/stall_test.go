package zapexporter

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/plog"
)

// deafPeer accepts connections and then does nothing with them: no handshake
// reply, no reads. It is what cloud looks like from a node while it restarts —
// the kernel completes the TCP handshake from the listen backlog long before
// the process is ready to answer anything.
//
// The connections are held, deliberately, so the socket never resets during the
// test. That is the state the fleet was actually in.
func deafPeer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var mu sync.Mutex
	var held []net.Conn
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			held = append(held, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		mu.Lock()
		for _, c := range held {
			_ = c.Close()
		}
		mu.Unlock()
	})
	return ln.Addr().String()
}

// TestADeafPeerDoesNotFreezeTheExporter is the regression test for the failure
// that cost this fleet hours of telemetry a day.
//
// luxfi/zap's TCP path sets no deadline anywhere: ConnectDirectID dials with a
// timeout and then blocks on a handshake read, and Conn.Send writes with none.
// send() used to hold one mutex across all of that, so a single call parked in
// the kernel stopped every consumer. The exporter then reported zero throughput
// AND zero errors, which is the one combination nothing downstream can recover
// from: exporterhelper never sees a failure, so it never retries, and the
// sending queue fills until it starts refusing.
//
// Measured on the busiest node, 135s after a cloud restart: 9,854 records
// accepted, 0 sent, 0 failed, queue at 197 MB of 256 MB and climbing. The agent
// had already logged "Connected to peer" — it looked healthy from every angle.
//
// The bar is simply that a send to a peer that never answers RETURNS, and
// returns an error, within the caller's deadline.
func TestADeafPeerDoesNotFreezeTheExporter(t *testing.T) {
	cfg := NewFactory().CreateDefaultConfig().(*Config)
	cfg.Endpoint = deafPeer(t)

	e := newExporter(exportertest.NewNopSettings(typeStr), cfg)
	if err := e.Start(context.Background(), componenttest.NewNopHost()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = e.Shutdown(context.Background()) }()

	ld := plog.NewLogs()
	ld.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().Body().SetStr("x")

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- e.pushLogs(ctx, ld) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a send to a peer that never answered reported success — the batch is gone and nothing will retry it")
		}
		if consumererror.IsPermanent(err) {
			t.Fatalf("a deaf peer is a transient condition and must stay retryable, got a permanent error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("pushLogs never returned: the exporter is wedged on a peer that never answered, which is the bug this test exists for")
	}
}

// TestOneStalledSendDoesNotBlockAnother states the property the old global
// mutex destroyed: consumers are independent. exporterhelper runs several, and
// one parked on a dead socket must not take the rest with it.
func TestOneStalledSendDoesNotBlockAnother(t *testing.T) {
	cfg := NewFactory().CreateDefaultConfig().(*Config)
	cfg.Endpoint = deafPeer(t)

	e := newExporter(exportertest.NewNopSettings(typeStr), cfg)
	if err := e.Start(context.Background(), componenttest.NewNopHost()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = e.Shutdown(context.Background()) }()

	ld := plog.NewLogs()
	ld.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().Body().SetStr("x")

	// One caller parks on a long deadline, as a consumer would.
	slow, cancelSlow := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancelSlow()
	go func() { _ = e.pushLogs(slow, ld) }()
	time.Sleep(100 * time.Millisecond) // let it get in first

	fast, cancelFast := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelFast()

	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- e.pushLogs(fast, ld) }()

	select {
	case <-done:
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("the second consumer waited %v on the first one's dead socket — they are still serialised", elapsed)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("a second consumer was held behind the first: one stalled peer still stops the whole exporter")
	}
}
