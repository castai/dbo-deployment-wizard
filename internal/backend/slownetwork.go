package backend

import (
	"context"
	"time"
)

// simulateSlowNetworkDelay is the per-call delay the --slow-network
// flag injects, to exercise the UI against a slow network locally.
const simulateSlowNetworkDelay = 2 * time.Second

// sleepCtx pauses for d, or until ctx is canceled.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
