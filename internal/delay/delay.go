// Package delay sleeps a random time so the jobs do not run at the same second every day.
package delay

import (
	"context"
	"math/rand/v2"
	"time"

	"go.uber.org/zap"
)

// Random sleeps for a random time between 0 and max, or until ctx is done (it then returns
// ctx.Err()). A max of zero or less does not sleep.
func Random(ctx context.Context, max time.Duration, log *zap.Logger) error {
	if max <= 0 {
		return nil
	}
	d := time.Duration(rand.Int64N(int64(max)))
	log.Info("delaying", zap.Duration("delay", d))
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
