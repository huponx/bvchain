package report

import (
	"testing"
	"time"

	"github.com/huponx/bvchain/internal/calendar"
)

func TestIdempotencyKey(t *testing.T) {
	now := time.Date(2026, 11, 19, 11, 17, 0, 0, calendar.VietnamLoc)
	period := calendar.PeriodAt(now)

	daily := idempotencyKey("", period, now)
	if daily != idempotencyKey(DedupeDaily, period, now) || daily != "bvreport-2026-10-21-2026-11-19" {
		t.Fatalf("daily = %q", daily)
	}
	if got := idempotencyKey(DedupeDaily, period, now.Add(5*time.Hour)); got != daily {
		t.Fatalf("daily key changes within a day: %q", got)
	}

	hourly := idempotencyKey(DedupeHourly, period, now)
	if hourly != "bvreport-2026-10-21-2026-11-19T11" {
		t.Fatalf("hourly = %q", hourly)
	}
	if idempotencyKey(DedupeHourly, period, now.Add(30*time.Minute)) != hourly {
		t.Fatal("hourly key changes within the hour")
	}
	if idempotencyKey(DedupeHourly, period, now.Add(time.Hour)) == hourly {
		t.Fatal("hourly key must change with the hour")
	}

	if got := idempotencyKey(DedupeNone, period, now); got != "" {
		t.Fatalf("none = %q, want no key", got)
	}
}
