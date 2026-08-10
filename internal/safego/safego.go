// Package safego provides panic-safe goroutine spawning and recovery for
// goroutines that run outside the request/response cycle.
//
// A panic that unwinds to the top of a bare goroutine calls os.Exit(2) and
// takes the whole process down — every connected WebSocket and every
// in-flight HTTP request on the instance, not just the one caller. chi's
// middleware.Recoverer only wraps the per-request goroutine, so anything we
// spawn ourselves (read/write pumps, background workers, heartbeats, fire-
// and-forget tasks like sending an email) is unprotected unless it goes
// through this package.
package safego

import (
	"log/slog"
	"runtime/debug"
)

// Go runs fn in a new goroutine with panic recovery. A panic inside fn is
// contained to that goroutine and logged with a stack trace instead of
// crashing the process.
func Go(log *slog.Logger, fn func()) {
	go func() {
		defer Recover(log, "goroutine")
		fn()
	}()
}

// Recover is a deferred recovery that logs a panic (with stack) under name.
// Use it directly (via `defer safego.Recover(log, "name")`) when a goroutine
// already exists and you want per-iteration recovery — e.g. an event worker
// that processes one item per loop and shouldn't die because one item was bad.
func Recover(log *slog.Logger, name string) {
	if r := recover(); r != nil {
		log.Error("recovered panic",
			slog.String("where", name),
			slog.Any("panic", r),
			slog.String("stack", string(debug.Stack())),
		)
	}
}
