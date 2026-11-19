package main

import (
	"testing"
	"time"

	"github.com/huponx/bvchain/internal/calendar"
)

func TestParseMonth(t *testing.T) {
	p, err := ParseMonth("10/2025")
	if err != nil {
		t.Fatal(err)
	}
	if calendar.DayKey(p.FromDate) != "2025-09-21" || calendar.DayKey(p.ToDate) != "2025-10-20" {
		t.Fatalf("%s..%s", calendar.DayKey(p.FromDate), calendar.DayKey(p.ToDate))
	}
	if p, _ = ParseMonth("01/2026"); calendar.DayKey(p.FromDate) != "2025-12-21" {
		t.Fatalf("january: %s", calendar.DayKey(p.FromDate))
	}
	for _, bad := range []string{"", "13/2025", "2025-10", "10/25", "oct"} {
		if _, err := ParseMonth(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestSample(t *testing.T) {
	now := time.Date(2026, 10, 5, 9, 0, 0, 0, calendar.VietnamLoc)
	id, data, err := sample("checkout", 2, "17:45", now)
	if err != nil || id != "bvchain_checkout" || data["shift"] != "2" || data["time"] != "17:45" {
		t.Fatalf("%q %v %v", id, data, err)
	}
	if _, data, _ = sample("checkin", 1, "", now); data["time"] != "09:00" {
		t.Fatalf("default time: %v", data)
	}
	for _, kind := range []string{"failed", "report"} {
		if _, d, err := sample(kind, 1, "", now); err != nil || len(d) == 0 {
			t.Fatalf("%s: %v", kind, err)
		}
	}
	if _, _, err := sample("checkin", 3, "", now); err == nil {
		t.Error("shift 3 accepted")
	}
	if _, _, err := sample("checkin", 1, "25:99", now); err == nil {
		t.Error("bad time accepted")
	}
	if _, _, err := sample("nope", 1, "", now); err == nil {
		t.Error("unknown kind accepted")
	}
}
