package report

import (
	"strings"
	"testing"
	"time"

	"github.com/huponx/bvchain/internal/calendar"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// The period 21/09/2026 to 20/10/2026 has 30 days and 8 weekend days: 22 working days.
var period = calendar.PeriodEndingIn(2026, time.October)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, calendar.VietnamLoc)
}

// fullMonth returns one item per date of the period, with weekends flagged and every working
// day checked in and out.
func fullMonth() []calendar.Item {
	var items []calendar.Item
	for t := period.From(); !t.After(period.To()); t = t.AddDate(0, 0, 1) {
		wk := t.Weekday() == time.Saturday || t.Weekday() == time.Sunday
		it := calendar.Item{TrackingDate: day(t.Year(), t.Month(), t.Day()).UnixMilli(), IsWeekend: wk}
		if !wk {
			it.TotalWorkingDay = 1
		}
		items = append(items, it)
	}
	return items
}

func find(items []calendar.Item, y int, m time.Month, d int) *calendar.Item {
	key := day(y, m, d).UnixMilli()
	for i := range items {
		if items[i].TrackingDate == key {
			return &items[i]
		}
	}
	return nil
}

var afterPeriod = day(2026, time.October, 21).Add(10 * time.Hour)

func summarize(items []calendar.Item, now time.Time) Summary {
	return Summarize(items, period, now, zap.NewNop())
}

func TestFullPeriodIs22WorkingDays(t *testing.T) {
	items := fullMonth()
	if len(items) != 30 {
		t.Fatalf("fixture has %d days", len(items))
	}
	s := summarize(items, afterPeriod)
	if s.TotalInPeriod != 22 || s.TotalToToday != 22 || s.DaysWithCICO != 22 || s.MissingDays != 0 {
		t.Fatalf("%+v", s)
	}
	if s.Weekend != 8 || s.LegacyTotalInPeriod != 22 || s.Duplicates != 0 || s.OutOfPeriod != 0 {
		t.Fatalf("%+v", s)
	}
}

// The old report counted every item that was not a weekend. The rows the API returned twice
// or outside the period inflated it to 24.
func TestDuplicatesAndOutOfPeriodDoNotInflateTheCount(t *testing.T) {
	items := fullMonth()
	items = append(items,
		*find(items, 2026, time.September, 22), // duplicate
		*find(items, 2026, time.October, 14),   // duplicate
	)
	s := summarize(items, afterPeriod)
	if s.LegacyTotalInPeriod != 24 {
		t.Fatalf("fixture does not reproduce the old count: %d", s.LegacyTotalInPeriod)
	}
	if s.TotalInPeriod != 22 || s.Duplicates != 2 || s.UniqueItems != 30 {
		t.Fatalf("%+v", s)
	}

	items = append(fullMonth(),
		calendar.Item{TrackingDate: day(2026, time.October, 21).UnixMilli()}, // after the period
		calendar.Item{TrackingDate: day(2026, time.September, 18).UnixMilli()},
	)
	s = summarize(items, afterPeriod)
	if s.LegacyTotalInPeriod != 24 || s.TotalInPeriod != 22 || s.OutOfPeriod != 2 {
		t.Fatalf("%+v", s)
	}
}

func TestDuplicateIsLoggedWithTheDifferingFields(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	items := fullMonth()
	dup := *find(items, 2026, time.September, 22)
	dup.TotalWorkingDay = 0.5
	dup.TimeIn = 30000
	items = append(items, dup)

	s := Summarize(items, period, afterPeriod, zap.New(core))
	if s.Duplicates != 1 {
		t.Fatalf("%+v", s)
	}
	warns := logs.FilterMessage("duplicate calendar item dropped").All()
	if len(warns) != 1 || warns[0].Level != zapcore.WarnLevel {
		t.Fatalf("warnings: %v", warns)
	}
	ctx := warns[0].ContextMap()
	if ctx["date"] != "2026-09-22" {
		t.Fatalf("date = %v", ctx["date"])
	}
	diff, _ := ctx["differs_in"].([]any)
	if len(diff) != 2 {
		t.Fatalf("differs_in = %v", ctx["differs_in"])
	}
	if logs.FilterMessage("working days summary").Len() != 1 {
		t.Fatal("missing the summary line")
	}
	// The first item of a date wins: the duplicate with 0.5 does not lower the credit.
	if s.DaysWithCICO != 22 {
		t.Fatalf("days with CI/CO = %v", s.DaysWithCICO)
	}
}

func TestHolidayAndLeaveAreNotWorkingDays(t *testing.T) {
	items := fullMonth()
	find(items, 2026, time.September, 22).IsHoliday = true
	find(items, 2026, time.September, 23).LeaveType = "annual" // no detail: full day
	find(items, 2026, time.September, 24).LeaveType = "annual"
	find(items, 2026, time.September, 24).LeaveDetail = []calendar.LeaveItem{{IsMorning: true}}
	for _, d := range []int{22, 23} {
		find(items, 2026, time.September, d).TotalWorkingDay = 0
	}
	find(items, 2026, time.September, 24).TotalWorkingDay = 0.5

	s := summarize(items, afterPeriod)
	if s.Holiday != 1 || s.FullLeave != 1 || s.HalfLeave != 1 {
		t.Fatalf("%+v", s)
	}
	if s.TotalInPeriod != 19.5 { // 22 - 1 - 1 - 0.5
		t.Fatalf("total in period = %v", s.TotalInPeriod)
	}
	if s.MissingDays != 0 {
		t.Fatalf("a half-day leave with its half worked is not missing: %+v", s)
	}
}

func TestMissingDaysCountOnlyUpToToday(t *testing.T) {
	items := fullMonth()
	// 2026-09-21 (Mon) .. 2026-09-25 (Fri) done, 09-28 (Mon) is not checked in, nothing after.
	for i := range items {
		if !items[i].IsWeekend && calendar.DayKey(items[i].TrackingDate) >= "2026-09-28" {
			items[i].TotalWorkingDay = 0
		}
	}
	now := day(2026, time.September, 29).Add(18 * time.Hour)
	s := summarize(items, now)

	// working days 21..25 (5) + 28, 29 (2) = 7 up to today; 5 covered; 28 and 29 missing
	if s.TotalToToday != 7 || s.DaysWithCICO != 5 || s.MissingDays != 2 || s.TotalInPeriod != 22 {
		t.Fatalf("%+v", s)
	}
	for _, d := range s.Days {
		if d.Date > "2026-09-29" && !d.Future {
			t.Fatalf("%s should be in the future", d.Date)
		}
	}
}

func TestPartialCheckInGivesPartialCredit(t *testing.T) {
	items := fullMonth()
	find(items, 2026, time.September, 21).TotalWorkingDay = 0.5
	now := day(2026, time.September, 21).Add(18 * time.Hour)
	s := summarize(items, now)
	if s.TotalToToday != 1 || s.DaysWithCICO != 0.5 || s.MissingDays != 0.5 {
		t.Fatalf("%+v", s)
	}
	if got := s.Data(period, now)["missing_days"]; got != "0.5" {
		t.Fatalf("missing_days = %q", got)
	}
}

func TestIsWeekendFlagWinsOverWeekday(t *testing.T) {
	// A working Saturday (flag false) stays a working day, and the mismatch is logged.
	core, logs := observer.New(zapcore.WarnLevel)
	items := fullMonth()
	find(items, 2026, time.September, 26).IsWeekend = false // Saturday
	s := Summarize(items, period, afterPeriod, zap.New(core))
	if s.TotalInPeriod != 23 || s.Weekend != 7 {
		t.Fatalf("%+v", s)
	}
	if logs.FilterMessageSnippet("disagrees with the weekday").Len() != 1 {
		t.Fatalf("logs: %v", logs.All())
	}
}

func TestDataAndTable(t *testing.T) {
	s := summarize(fullMonth(), day(2026, time.October, 19).Add(18*time.Hour))
	data := s.Data(period, day(2026, time.October, 19))
	want := map[string]string{
		"from_date": "21/09", "to_date": "20/10", "today": "19/10",
		"total_in_period": "22", "total_to_today": "21", "days_with_cico": "21", "missing_days": "00",
	}
	for k, v := range want {
		if data[k] != v {
			t.Errorf("%s = %q, want %q", k, data[k], v)
		}
	}
	table := s.Table()
	if !strings.Contains(table, "2026-09-26") || !strings.Contains(table, ClassWeekend) {
		t.Fatalf("table:\n%s", table)
	}
}
