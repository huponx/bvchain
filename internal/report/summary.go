// Package report counts the working days of a billing period and sends the working report.
package report

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/huponx/bvchain/internal/calendar"
	"go.uber.org/zap"
)

// Day classes (Day.Class).
const (
	ClassWorking     = "working"
	ClassWeekend     = "weekend"
	ClassHoliday     = "holiday"
	ClassLeave       = "leave"         // the whole day is leave
	ClassOutOfPeriod = "out_of_period" // the API returned a day outside the period
)

// Day is how one calendar date was classified.
type Day struct {
	Date    string // YYYY-MM-DD (UTC+7)
	Weekday time.Weekday
	Class   string
	// Weight is the part of the day that is a working day: 1, 0.5 for a half-day leave, 0 for
	// a weekend, holiday or full-day leave.
	Weight float64
	// Credit is the part of Weight covered by check-in/check-out (up to today).
	Credit float64
	// LeaveShare is the part of the day recorded as leave.
	LeaveShare float64
	// TotalWorkingDay is the value of the API (1.0 = full day with CI/CO).
	TotalWorkingDay float64
	IsWeekend       bool
	IsHoliday       bool
	LeaveType       string
	Future          bool // after today: not counted in TotalToToday
}

// Summary is the result of Summarize.
type Summary struct {
	// What the API returned.
	RawItems    int // items returned
	UniqueItems int // distinct dates
	Duplicates  int // items dropped because their date was already seen
	OutOfPeriod int // distinct dates outside the period (dropped)

	// Classification of the dates of the period.
	Weekend   int
	Holiday   int
	FullLeave int // full-day leave
	HalfLeave int // days with a half-day leave

	// LegacyTotalInPeriod is what the old report counted (every item that is not a weekend):
	// kept in the log to compare with TotalInPeriod.
	LegacyTotalInPeriod int

	// TotalInPeriod is the working days of the period, TotalToToday up to today (inclusive),
	// DaysWithCICO the part of them covered by check-in/check-out, MissingDays the rest.
	TotalInPeriod float64
	TotalToToday  float64
	DaysWithCICO  float64
	MissingDays   float64

	Days []Day // one entry per distinct date of the period, ordered by date
}

// Summarize counts the working days of the period from the calendar items.
//
// The API items are first de-duplicated by date (the first item of a date wins, a warning
// lists the fields that differ), then the dates outside the period are dropped, then every
// date is classified: weekend (is_weekend), holiday (is_holiday), leave (leave_type or the
// submitted-leave flags, per shift), working otherwise. A half-day leave counts 0.5.
//
// Every step is logged: warnings for duplicates, out-of-period dates and an is_weekend flag
// that disagrees with the weekday, debug lines for each date, and one info line with totals.
func Summarize(items []calendar.Item, period calendar.DateRange, now time.Time, log *zap.Logger) Summary {
	from, to := calendar.DayKey(period.FromDate), calendar.DayKey(period.ToDate)
	today := now.In(calendar.VietnamLoc).Format("2006-01-02")

	s := Summary{RawItems: len(items)}
	for _, it := range items {
		if !it.IsWeekend {
			s.LegacyTotalInPeriod++
		}
	}

	// De-duplicate by date: the first item wins.
	first := map[string]calendar.Item{}
	var order []string
	for _, it := range items {
		key := calendar.DayKey(it.TrackingDate)
		prev, seen := first[key]
		if !seen {
			first[key] = it
			order = append(order, key)
			continue
		}
		s.Duplicates++
		log.Warn("duplicate calendar item dropped",
			zap.String("date", key),
			zap.Int64("tracking_date", it.TrackingDate),
			zap.Strings("differs_in", diffFields(prev, it)),
		)
	}
	sort.Strings(order)
	s.UniqueItems = len(order)

	for _, key := range order {
		it := first[key]
		wd := time.UnixMilli(it.TrackingDate).In(calendar.VietnamLoc).Weekday()
		d := Day{
			Date: key, Weekday: wd, TotalWorkingDay: it.TotalWorkingDay,
			IsWeekend: it.IsWeekend, IsHoliday: it.IsHoliday, LeaveType: it.LeaveType,
			Future: key > today,
		}

		if key < from || key > to {
			d.Class = ClassOutOfPeriod
			s.OutOfPeriod++
			log.Warn("calendar item outside the period dropped", zap.String("date", key), zap.String("period", from+".."+to))
			logDay(log, d)
			continue
		}

		if weekendByDay := wd == time.Saturday || wd == time.Sunday; weekendByDay != it.IsWeekend {
			log.Warn("is_weekend disagrees with the weekday (the API flag is used)",
				zap.String("date", key), zap.String("weekday", wd.String()), zap.Bool("is_weekend", it.IsWeekend))
		}

		d.LeaveShare = calendar.LeaveShare(it)
		switch {
		case it.IsWeekend:
			d.Class = ClassWeekend
			s.Weekend++
		case it.IsHoliday:
			d.Class = ClassHoliday
			s.Holiday++
		case d.LeaveShare >= 1:
			d.Class = ClassLeave
			s.FullLeave++
		default:
			d.Class = ClassWorking
			d.Weight = 1 - d.LeaveShare
			if d.LeaveShare > 0 {
				s.HalfLeave++
			}
		}

		if d.Weight > 0 {
			s.TotalInPeriod += d.Weight
			if !d.Future {
				s.TotalToToday += d.Weight
				d.Credit = min(it.TotalWorkingDay, d.Weight)
				s.DaysWithCICO += d.Credit
			}
		}
		s.Days = append(s.Days, d)
		logDay(log, d)
	}
	s.MissingDays = s.TotalToToday - s.DaysWithCICO

	log.Info("working days summary",
		zap.String("period", from+".."+to),
		zap.String("today", today),
		zap.Int("raw_items", s.RawItems),
		zap.Int("unique_dates", s.UniqueItems),
		zap.Int("duplicates", s.Duplicates),
		zap.Int("out_of_period", s.OutOfPeriod),
		zap.Int("weekend", s.Weekend),
		zap.Int("holiday", s.Holiday),
		zap.Int("full_leave", s.FullLeave),
		zap.Int("half_leave", s.HalfLeave),
		zap.Int("legacy_total_in_period", s.LegacyTotalInPeriod),
		zap.Float64("total_in_period", s.TotalInPeriod),
		zap.Float64("total_to_today", s.TotalToToday),
		zap.Float64("days_with_cico", s.DaysWithCICO),
		zap.Float64("missing_days", s.MissingDays),
	)
	return s
}

func logDay(log *zap.Logger, d Day) {
	log.Debug("calendar day",
		zap.String("date", d.Date),
		zap.String("weekday", d.Weekday.String()),
		zap.String("class", d.Class),
		zap.Bool("is_weekend", d.IsWeekend),
		zap.Bool("is_holiday", d.IsHoliday),
		zap.String("leave_type", d.LeaveType),
		zap.Float64("leave_share", d.LeaveShare),
		zap.Float64("total_working_day", d.TotalWorkingDay),
		zap.Float64("weight", d.Weight),
		zap.Float64("credit", d.Credit),
		zap.Bool("future", d.Future),
	)
}

// diffFields lists the fields in which two items of the same date differ.
func diffFields(a, b calendar.Item) []string {
	var f []string
	add := func(name string, differs bool) {
		if differs {
			f = append(f, name)
		}
	}
	add("time_sheet_id", a.TimeSheetID != b.TimeSheetID)
	add("tracking_date", a.TrackingDate != b.TrackingDate)
	add("time_in", a.TimeIn != b.TimeIn)
	add("time_out", a.TimeOut != b.TimeOut)
	add("total_working_day", a.TotalWorkingDay != b.TotalWorkingDay)
	add("is_weekend", a.IsWeekend != b.IsWeekend)
	add("is_holiday", a.IsHoliday != b.IsHoliday)
	add("leave_type", a.LeaveType != b.LeaveType)
	add("leave_detail", len(a.LeaveDetail) != len(b.LeaveDetail))
	add("work_shifts", len(a.WorkShifts) != len(b.WorkShifts))
	return f
}

// Data is the template data of the bvreport_working_report message.
func (s Summary) Data(period calendar.DateRange, now time.Time) map[string]string {
	return map[string]string{
		"from_date":       period.From().Format("02/01"),
		"to_date":         period.To().Format("02/01"),
		"today":           now.In(calendar.VietnamLoc).Format("02/01"),
		"total_in_period": days(s.TotalInPeriod),
		"total_to_today":  days(s.TotalToToday),
		"days_with_cico":  days(s.DaysWithCICO),
		"missing_days":    days(s.MissingDays),
	}
}

// days formats a day count: "22", or "21.5" for half days.
func days(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%02d", int64(v))
	}
	return fmt.Sprintf("%.1f", v)
}

// Table is a readable, per-date listing of the classification (bvctl -verbose).
func (s Summary) Table() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%-10s %-4s %-13s %-6s %-6s %-5s %s\n", "date", "day", "class", "weight", "credit", "twd", "notes")
	for _, d := range s.Days {
		var notes []string
		if d.IsWeekend {
			notes = append(notes, "is_weekend")
		}
		if d.IsHoliday {
			notes = append(notes, "is_holiday")
		}
		if d.LeaveType != "" {
			notes = append(notes, "leave="+d.LeaveType)
		}
		if d.LeaveShare > 0 {
			notes = append(notes, fmt.Sprintf("leave_share=%.1f", d.LeaveShare))
		}
		if d.Future {
			notes = append(notes, "future")
		}
		fmt.Fprintf(&b, "%-10s %-4s %-13s %-6.1f %-6.1f %-5.1f %s\n",
			d.Date, d.Weekday.String()[:3], d.Class, d.Weight, d.Credit, d.TotalWorkingDay, strings.Join(notes, " "))
	}
	return b.String()
}
