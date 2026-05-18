package api

import (
	"errors"
	"io"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// blockingReader returns one chunk per Read call, blocking until either
// the chunk is delivered, the reader is Closed, or the test signals via
// release. It tracks concurrent Read invocations so the test can assert
// that no two Reads ever run against the same underlying body.
type blockingReader struct {
	chunks  chan []byte
	closed  chan struct{}
	inFlight int32
	maxConcurrent int32
}

func newBlockingReader() *blockingReader {
	return &blockingReader{
		chunks: make(chan []byte, 16),
		closed: make(chan struct{}),
	}
}

func (b *blockingReader) Read(p []byte) (int, error) {
	c := atomic.AddInt32(&b.inFlight, 1)
	defer atomic.AddInt32(&b.inFlight, -1)
	for {
		m := atomic.LoadInt32(&b.maxConcurrent)
		if c <= m || atomic.CompareAndSwapInt32(&b.maxConcurrent, m, c) {
			break
		}
	}
	select {
	case chunk, ok := <-b.chunks:
		if !ok {
			return 0, io.EOF
		}
		n := copy(p, chunk)
		return n, nil
	case <-b.closed:
		return 0, io.ErrClosedPipe
	}
}

func (b *blockingReader) Close() error {
	select {
	case <-b.closed:
		// already closed
	default:
		close(b.closed)
	}
	return nil
}

func (b *blockingReader) feed(data string) { b.chunks <- []byte(data) }

// TestIdleTimeoutReader_ResetExtendsDeadline verifies that data arriving
// before the idle timeout resets the deadline and does not produce a
// spurious timeout error on subsequent reads. The previous implementation
// called timer.Reset without first stopping and draining the channel,
// causing a stale tick to fire the timeout case immediately on the next
// Read even when fresh data was arriving.
func TestIdleTimeoutReader_ResetExtendsDeadline(t *testing.T) {
	br := newBlockingReader()
	r := newIdleTimeoutReader(br, 100*time.Millisecond)
	defer r.Close()

	// Feed several chunks at intervals shorter than the timeout; the
	// reader should successfully consume all of them.
	go func() {
		for i := 0; i < 5; i++ {
			time.Sleep(40 * time.Millisecond)
			br.feed("chunk")
		}
		// Allow the consumer's final Read to return before closing.
		time.Sleep(40 * time.Millisecond)
		br.Close()
	}()

	buf := make([]byte, 64)
	got := 0
	for {
		n, err := r.Read(buf)
		got += n
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
				break
			}
			t.Fatalf("unexpected error after %d bytes: %v", got, err)
		}
	}
	if got != 5*len("chunk") {
		t.Fatalf("got %d bytes, want %d", got, 5*len("chunk"))
	}
}

// TestIdleTimeoutReader_FiresOnIdle verifies that the idle timeout
// triggers when no data arrives and that the in-flight Read unblocks
// promptly (rather than leaving an orphan goroutine, which was the
// previous implementation's failure mode).
func TestIdleTimeoutReader_FiresOnIdle(t *testing.T) {
	br := newBlockingReader()
	r := newIdleTimeoutReader(br, 50*time.Millisecond)
	defer r.Close()

	gBefore := runtime.NumGoroutine()

	buf := make([]byte, 64)
	start := time.Now()
	_, err := r.Read(buf)
	elapsed := time.Since(start)

	if err == nil || !strings.Contains(err.Error(), "idle timeout") {
		t.Fatalf("want idle timeout error, got %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("Read took %v, expected ~50ms", elapsed)
	}

	// Give the runtime a moment to reap any goroutines that the Read
	// might have spawned. The new implementation spawns none; the old
	// one orphaned one goroutine per Read call.
	time.Sleep(50 * time.Millisecond)
	gAfter := runtime.NumGoroutine()
	if gAfter > gBefore+1 {
		t.Fatalf("goroutine leak: before=%d after=%d", gBefore, gAfter)
	}
}

// TestIdleTimeoutReader_NoConcurrentBodyReads exercises many Read calls
// against a reader whose chunks arrive at intervals close to the idle
// timeout. The original code spawned a goroutine per Read; if the timer
// fired spuriously the parent returned but the orphan goroutine kept
// reading the body, producing two concurrent Reads on the same body.
// Here we assert the underlying body never sees more than one in-flight
// Read at any time.
func TestIdleTimeoutReader_NoConcurrentBodyReads(t *testing.T) {
	br := newBlockingReader()
	r := newIdleTimeoutReader(br, 30*time.Millisecond)
	defer r.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 20; i++ {
			time.Sleep(15 * time.Millisecond)
			br.feed("x")
		}
		br.Close()
	}()

	buf := make([]byte, 8)
	for {
		_, err := r.Read(buf)
		if err != nil {
			break
		}
	}
	<-done

	if max := atomic.LoadInt32(&br.maxConcurrent); max > 1 {
		t.Fatalf("body saw %d concurrent reads; want 1", max)
	}
}

// TestIdleTimeoutReader_CloseUnblocks verifies that calling Close on
// the wrapper unblocks an in-flight Read by closing the underlying
// body.
func TestIdleTimeoutReader_CloseUnblocks(t *testing.T) {
	br := newBlockingReader()
	r := newIdleTimeoutReader(br, time.Hour)

	errCh := make(chan error, 1)
	go func() {
		buf := make([]byte, 8)
		_, err := r.Read(buf)
		errCh <- err
	}()

	time.Sleep(20 * time.Millisecond)
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatalf("expected error after Close")
		}
	case <-time.After(time.Second):
		t.Fatalf("Read did not unblock after Close")
	}
}
