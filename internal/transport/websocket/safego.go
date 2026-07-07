package websocket

import (
	"log/slog"
	"runtime/debug"
)

// safeGo runs fn in a new goroutine with panic recovery.
//
// A panic that unwinds to the top of a bare goroutine calls os.Exit(2) and
// takes the whole process down — every connected WebSocket and every in-flight
// HTTP request on the instance, not just the one connection. chi's
// middleware.Recoverer only wraps the per-request goroutine, so goroutines we
// spawn ourselves (read/write pumps, the event worker, heartbeats, the
// backpressure-unregister) are unprotected. Routing them through safeGo contains
// a panic to its own goroutine and logs it with a stack instead of crashing the
// node.
func safeGo(log *slog.Logger, fn func()) {
	go func() {
		defer recoverPanic(log, "ws goroutine")
		fn()
	}()
}

// recoverPanic is a deferred recovery that logs a panic (with stack) under name.
// Used both by safeGo and inline where a goroutine already exists (the event
// worker recovers per-event so one bad message can't kill the worker).
func recoverPanic(log *slog.Logger, name string) {
	if r := recover(); r != nil {
		log.Error("recovered panic",
			slog.String("where", name),
			slog.Any("panic", r),
			slog.String("stack", string(debug.Stack())),
		)
	}
}
