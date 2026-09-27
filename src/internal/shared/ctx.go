package shared

import (
	"context"
	"errors"
	"time"
)

// Deadlines for OpenStack API calls. The HTTP transport bounds only dial,
// TLS handshake and response-header time, so without a context deadline a
// stalled response body (or a retry wait) could hang a command forever.
// They are variables so tests can shorten them.
var (
	// RequestTimeout bounds a single request-scoped command.
	RequestTimeout = 30 * time.Second
	// LongRequestTimeout bounds multi-step operations and wait loops.
	LongRequestTimeout = 5 * time.Minute
	// TransferStallTimeout is how long a streaming transfer may make no
	// progress before it is abandoned.
	TransferStallTimeout = 2 * time.Minute
)

// ErrTransferStalled is the cancellation cause of a StallCtx context whose
// transfer stopped making progress.
var ErrTransferStalled = errors.New("transfer stalled")

// RequestCtx returns a context bounded by RequestTimeout.
func RequestCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), RequestTimeout)
}

// LongRequestCtx returns a context bounded by LongRequestTimeout.
func LongRequestCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), LongRequestTimeout)
}

// StallCtx returns a context for a streaming transfer of unknown duration.
// It has no overall deadline, so a large but healthy transfer is never cut
// off, but it is canceled (with cause ErrTransferStalled) once progress,
// a monotonic byte counter, has not changed for TransferStallTimeout.
func StallCtx(progress func() int64) (context.Context, context.CancelFunc) {
	idle := TransferStallTimeout
	ctx, cancel := context.WithCancelCause(context.Background())
	go func() {
		tick := time.NewTicker(max(idle/8, time.Millisecond))
		defer tick.Stop()
		last, lastMoved := progress(), time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-tick.C:
				if p := progress(); p != last {
					last, lastMoved = p, now
				} else if now.Sub(lastMoved) >= idle {
					cancel(ErrTransferStalled)
					return
				}
			}
		}
	}()
	return ctx, func() { cancel(context.Canceled) }
}
