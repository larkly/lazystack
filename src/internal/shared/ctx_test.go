package shared

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRequestCtxHasDeadlines(t *testing.T) {
	for name, tc := range map[string]struct {
		fn   func() (context.Context, context.CancelFunc)
		want time.Duration
	}{
		"request": {RequestCtx, RequestTimeout},
		"long":    {LongRequestCtx, LongRequestTimeout},
	} {
		ctx, cancel := tc.fn()
		dl, ok := ctx.Deadline()
		cancel()
		if !ok {
			t.Fatalf("%s: no deadline", name)
		}
		if left := time.Until(dl); left <= 0 || left > tc.want {
			t.Fatalf("%s: deadline in %v, want within %v", name, left, tc.want)
		}
	}
}

func TestStallCtxCancelsOnlyStalledTransfers(t *testing.T) {
	orig := TransferStallTimeout
	TransferStallTimeout = 100 * time.Millisecond
	t.Cleanup(func() { TransferStallTimeout = orig })

	// A transfer that keeps moving outlives many stall windows.
	var moved atomic.Int64
	ctx, cancel := StallCtx(moved.Load)
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(5 * time.Millisecond):
				moved.Add(1)
			}
		}
	}()
	select {
	case <-ctx.Done():
		t.Fatalf("progressing transfer canceled: %v", context.Cause(ctx))
	case <-time.After(5 * TransferStallTimeout):
	}
	close(stop)

	// Once it stops moving it is canceled with the stall cause.
	select {
	case <-ctx.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("stalled transfer was never canceled")
	}
	if !errors.Is(context.Cause(ctx), ErrTransferStalled) {
		t.Fatalf("cause = %v, want ErrTransferStalled", context.Cause(ctx))
	}
	cancel()

	// An explicit cancel is not reported as a stall.
	ctx, cancel = StallCtx(func() int64 { return 0 })
	cancel()
	if errors.Is(context.Cause(ctx), ErrTransferStalled) {
		t.Fatal("explicit cancel reported as a stall")
	}
	if _, ok := ctx.Deadline(); ok {
		t.Fatal("stall context must not carry an overall deadline")
	}
}
