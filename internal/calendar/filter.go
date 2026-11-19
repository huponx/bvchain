package calendar

import "time"

// CurrentSessions returns only the sessions whose tracking_date falls on today (UTC+7).
func CurrentSessions(sessions []WorkSession) []WorkSession {
	now := time.Now().In(vietnamLoc)
	y, m, d := now.Date()

	// Start/end of day (millis)
	startOfDay := time.Date(y, m, d, 0, 0, 0, 0, vietnamLoc).UnixMilli()
	endOfDay := startOfDay + 24*60*60*1000 - 1

	hour, _, _ := now.Clock()

	// Each window extends to end-of-day past the checkout boundary so that a
	// cron run arriving late (e.g. 18:00 instead of 17:30) still fires checkout.
	// ShouldCheckOut gates the actual checkout call; the window just keeps the
	// session visible until midnight.
	var workShift int
	switch {
	case hour >= 8 && hour < 13: // 08:00–12:59 → morning
		workShift = 1
	case hour >= 13: // 13:00–23:59 → afternoon
		workShift = 2
	default:
		return nil
	}

	// Pre-allocate slice
	result := make([]WorkSession, 0, 2)

	for _, s := range sessions {
		if s.WorkShift == workShift &&
			s.TrackingDate >= startOfDay &&
			s.TrackingDate <= endOfDay {
			result = append(result, s)
		}
	}

	return result
}

// ShouldCheckOut reports whether it is past the checkout boundary for the given shift.
//   - Shift 1 (morning):   check out from 12:00 onwards
//   - Shift 2 (afternoon): check out from 17:30 onwards
func ShouldCheckOut(shift int) bool {
	now := time.Now().In(vietnamLoc)
	h, m, _ := now.Clock()
	switch shift {
	case 1:
		return h >= 12
	case 2:
		return h > 17 || (h == 17 && m >= 30)
	}
	return false
}

// FilterWorkSessions returns the non-excluded shifts from items.
//
// Excluded:
//   - Weekends (is_weekend)
//   - Holidays (is_holiday)
//   - Leave shifts (derived from leave_detail; full item excluded when no detail exists)
//   - Offshore shifts (current_work_mode.name == "Offshore" || request_work_mode.name == "Offshore")
func FilterWorkSessions(items []Item) []WorkSession {
	var sessions []WorkSession
	for _, item := range items {
		if item.IsWeekend || item.IsHoliday {
			continue
		}
		for _, ws := range item.WorkShifts {
			if ws.CurrentWorkMode.Name == "Offshore" || ws.RequestWorkMode.Name == "Offshore" {
				continue
			}
			if isLeaveShift(item, ws.WorkShift) {
				continue
			}
			var alreadyCheckedOut bool
			switch ws.WorkShift {
			case 1:
				alreadyCheckedOut = item.TotalWorkingDayMorning >= 0.5
			case 2:
				alreadyCheckedOut = item.TotalWorkingDayAfternoon >= 0.5
			}
			sessions = append(sessions, WorkSession{
				ScheduleID:        ws.ScheduleID,
				TrackingDate:      item.TrackingDate,
				WorkShift:         ws.WorkShift,
				WorkMode:          ws.CurrentWorkMode,
				TimeIn:            item.TimeIn,
				AlreadyCheckedOut: alreadyCheckedOut,
			})
		}
	}
	return sessions
}

// isLeaveShift reports whether the given shift (1=morning, 2=afternoon) is a leave shift
// for this item. It checks leave_type / submit flags first, then uses leave_detail to
// narrow down to the specific shift; if no detail exists the entire day is treated as leave.
func isLeaveShift(item Item, shift int) bool {
	hasLeave := item.LeaveType != "" || item.IsSubmitLeaveRequest || item.IsSubmitPaidLeaveRequest
	if !hasLeave {
		return false
	}
	if len(item.LeaveDetail) == 0 {
		return true // no per-shift detail → full-day leave
	}
	for _, ld := range item.LeaveDetail {
		if ld.IsFullDay {
			return true
		}
		if shift == 1 && ld.IsMorning {
			return true
		}
		if shift == 2 && ld.IsAfternoon {
			return true
		}
	}
	return false
}

// LeaveShare is the part of the day (0, 0.5 or 1) that the item records as leave: each of the
// morning and the afternoon shift counts 0.5 when isLeaveShift says so.
func LeaveShare(item Item) float64 {
	share := 0.0
	if isLeaveShift(item, 1) {
		share += 0.5
	}
	if isLeaveShift(item, 2) {
		share += 0.5
	}
	return share
}

// DayKey is the calendar date (UTC+7) of a millisecond timestamp, as YYYY-MM-DD.
func DayKey(ms int64) string {
	return time.UnixMilli(ms).In(vietnamLoc).Format("2006-01-02")
}
