package calendar

// Response is the top-level API response.
type Response struct {
	Code string `json:"code"`
	Data struct {
		Items []Item `json:"items"`
	} `json:"data"`
}

// Item represents one day in the calendar.
type Item struct {
	TimeSheetID              string      `json:"time_sheet_id"`
	TrackingDate             int64       `json:"tracking_date"`               // Unix ms
	TimeIn                   int         `json:"time_in"`                     // seconds since midnight; 0 = not checked in
	TimeOut                  int         `json:"time_out"`                    // seconds since midnight; 0 = not checked out
	TotalWorkingDay          float64     `json:"total_working_day"`           // 1.0 = full day with CI/CO
	TotalWorkingDayMorning   float64     `json:"total_working_day_morning"`   // 0.5 = morning checkout done
	TotalWorkingDayAfternoon float64     `json:"total_working_day_afternoon"` // 0.5 = afternoon checkout done
	IsWeekend                bool        `json:"is_weekend"`
	IsHoliday                bool        `json:"is_holiday"`
	LeaveType                string      `json:"leave_type"`
	IsSubmitLeaveRequest     bool        `json:"is_submit_leave_request"`
	IsSubmitPaidLeaveRequest bool        `json:"is_submit_paid_leave_request"`
	LeaveDetail              []LeaveItem `json:"leave_detail"`
	WorkShifts               []WorkShift `json:"work_shifts"`
}

// LeaveItem describes which parts of a day are on leave.
type LeaveItem struct {
	IsMorning   bool `json:"is_morning"`
	IsAfternoon bool `json:"is_afternoon"`
	IsFullDay   bool `json:"is_full_day"`
}

// WorkShift represents a half-day shift (1=morning, 2=afternoon).
type WorkShift struct {
	ScheduleID      string   `json:"schedule_id"`
	WorkDate        int64    `json:"work_date"`
	WorkShift       int      `json:"work_shift"`
	CurrentWorkMode WorkMode `json:"current_work_mode"`
	RequestWorkMode WorkMode `json:"request_work_mode"`
}

// WorkMode is the mode assigned to a shift.
type WorkMode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// WorkSession is a filtered result: one non-excluded shift.
type WorkSession struct {
	ScheduleID        string
	TrackingDate      int64
	WorkShift         int // 1=morning, 2=afternoon
	WorkMode          WorkMode
	TimeIn            int  // seconds since midnight; 0 = not checked in yet
	AlreadyCheckedOut bool // true when total_working_day_{morning|afternoon} >= 0.5
}
