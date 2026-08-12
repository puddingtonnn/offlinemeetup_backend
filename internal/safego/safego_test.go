package safego

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

func newTestLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

// notifyWriter wraps a buffer and signals a channel the first time it's
// written to. Used to establish a happens-before edge between a log write
// happening on a background goroutine (e.g. inside safego.Recover) and the
// test goroutine reading the buffer afterwards — reading buf.String() right
// after a WaitGroup.Wait() from the *panicking* goroutine is not enough,
// since that WaitGroup is signaled by the panicking function's own defer,
// which runs before Recover (registered by Go, above fn) gets to log.
type notifyWriter struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	done chan struct{}
	once sync.Once
}

func newNotifyWriter() *notifyWriter {
	return &notifyWriter{done: make(chan struct{})}
}

func (w *notifyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	n, err := w.buf.Write(p)
	w.mu.Unlock()
	w.once.Do(func() { close(w.done) })
	return n, err
}

func TestGo_RunsFunction(t *testing.T) {
	log, _ := newTestLogger()

	var wg sync.WaitGroup
	wg.Add(1)
	ran := false
	Go(log, func() {
		defer wg.Done()
		ran = true
	})

	wg.Wait()
	if !ran {
		t.Fatal("expected fn to run")
	}
}

func TestGo_RecoversPanicAndLogs(t *testing.T) {
	w := newNotifyWriter()
	log := slog.New(slog.NewTextHandler(w, nil))

	Go(log, func() {
		panic("boom")
	})

	<-w.done // wait until the panic has been logged

	w.mu.Lock()
	out := w.buf.String()
	w.mu.Unlock()

	if !strings.Contains(out, "recovered panic") {
		t.Fatalf("expected log to contain recovered panic, got: %q", out)
	}
	if !strings.Contains(out, "boom") {
		t.Fatalf("expected log to contain panic value, got: %q", out)
	}
}

func TestRecover_NoPanicIsNoop(t *testing.T) {
	log, buf := newTestLogger()

	func() {
		defer Recover(log, "test")
	}()

	if buf.Len() != 0 {
		t.Fatalf("expected no log output, got: %q", buf.String())
	}
}

func TestRecover_LogsPanicWithName(t *testing.T) {
	log, buf := newTestLogger()

	func() {
		defer Recover(log, "my-worker")
		panic("kaboom")
	}()

	out := buf.String()
	if !strings.Contains(out, "my-worker") {
		t.Fatalf("expected log to contain the scope name, got: %q", out)
	}
	if !strings.Contains(out, "kaboom") {
		t.Fatalf("expected log to contain the panic value, got: %q", out)
	}
}
