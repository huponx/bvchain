package delay

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestRandom(t *testing.T) {
	log := zap.NewNop()
	if err := Random(context.Background(), 0, log); err != nil {
		t.Fatalf("zero delay: %v", err)
	}
	if err := Random(context.Background(), -time.Second, log); err != nil {
		t.Fatalf("negative delay: %v", err)
	}
	start := time.Now()
	if err := Random(context.Background(), 20*time.Millisecond, log); err != nil || time.Since(start) > time.Second {
		t.Fatalf("short delay: %v after %v", err, time.Since(start))
	}
}

func TestRandomStopsOnContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// a one hour bound: the call must return at once, and the sleep can be 0 only by chance
	// (then no error), so retry a few times until a non-zero sleep is drawn.
	for i := 0; i < 20; i++ {
		err := Random(ctx, time.Hour, zap.NewNop())
		if errors.Is(err, context.Canceled) {
			return
		}
	}
	t.Fatal("a cancelled context never interrupted the delay")
}
