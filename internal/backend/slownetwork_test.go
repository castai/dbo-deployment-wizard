package backend

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSleepCtx(t *testing.T) {
	t.Run("sleeps through the duration", func(t *testing.T) {
		start := time.Now()
		if err := sleepCtx(context.Background(), 50*time.Millisecond); err != nil {
			t.Fatalf("sleepCtx() error = %v, want nil", err)
		}
		if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
			t.Fatalf("sleepCtx() returned after %v, want at least 50ms", elapsed)
		}
	})

	t.Run("canceled context returns at once", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		start := time.Now()
		if err := sleepCtx(ctx, time.Minute); !errors.Is(err, context.Canceled) {
			t.Fatalf("sleepCtx() error = %v, want %v", err, context.Canceled)
		}
		if elapsed := time.Since(start); elapsed >= time.Second {
			t.Fatalf("sleepCtx() took %v on a canceled context, want immediate return", elapsed)
		}
	})
}
