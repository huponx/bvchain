package calendar

import (
	"testing"
	"time"
)

func TestPeriods(t *testing.T) {
	at := func(y int, m time.Month, d, h int) time.Time { return time.Date(y, m, d, h, 0, 0, 0, VietnamLoc) }
	cases := []struct {
		name     string
		now      time.Time
		from, to string
	}{
		{"before the 21st", at(2025, time.October, 5, 9), "2025-09-21", "2025-10-20"},
		{"the 20th", at(2025, time.October, 20, 23), "2025-09-21", "2025-10-20"},
		{"the 21st", at(2025, time.October, 21, 0), "2025-10-21", "2025-11-20"},
		{"december rolls the year", at(2025, time.December, 25, 9), "2025-12-21", "2026-01-20"},
		{"january", at(2026, time.January, 3, 9), "2025-12-21", "2026-01-20"},
		// The old implementation used now.AddDate(0, 1, 0): on the 29th to 31st the next month
		// overflowed (31/10 + 1 month = 1/12) and the period ended in December.
		{"31st of a 31-day month", at(2026, time.October, 31, 9), "2026-10-21", "2026-11-20"},
		{"31st of august", at(2026, time.August, 31, 9), "2026-08-21", "2026-09-20"},
		{"31st of january", at(2026, time.January, 31, 9), "2026-01-21", "2026-02-20"},
		{"30th of january", at(2026, time.January, 30, 9), "2026-01-21", "2026-02-20"},
		{"29th of january", at(2027, time.January, 29, 9), "2027-01-21", "2027-02-20"},
	}
	for _, c := range cases {
		p := PeriodAt(c.now)
		if got := DayKey(p.FromDate); got != c.from {
			t.Errorf("%s: from = %s, want %s", c.name, got, c.from)
		}
		if got := DayKey(p.ToDate); got != c.to {
			t.Errorf("%s: to = %s, want %s", c.name, got, c.to)
		}
	}
	p := PeriodEndingIn(2025, time.October)
	if DayKey(p.FromDate) != "2025-09-21" || DayKey(p.ToDate) != "2025-10-20" {
		t.Errorf("PeriodEndingIn(2025, October) = %s..%s", DayKey(p.FromDate), DayKey(p.ToDate))
	}
}

func TestLeaveShare(t *testing.T) {
	cases := []struct {
		name string
		item Item
		want float64
	}{
		{"no leave", Item{}, 0},
		{"leave without detail is a full day", Item{LeaveType: "annual"}, 1},
		{"full day detail", Item{LeaveType: "annual", LeaveDetail: []LeaveItem{{IsFullDay: true}}}, 1},
		{"morning only", Item{LeaveType: "annual", LeaveDetail: []LeaveItem{{IsMorning: true}}}, 0.5},
		{"afternoon only", Item{IsSubmitLeaveRequest: true, LeaveDetail: []LeaveItem{{IsAfternoon: true}}}, 0.5},
		{"both halves", Item{LeaveType: "annual", LeaveDetail: []LeaveItem{{IsMorning: true}, {IsAfternoon: true}}}, 1},
	}
	for _, c := range cases {
		if got := LeaveShare(c.item); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
