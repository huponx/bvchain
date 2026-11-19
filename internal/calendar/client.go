// Package calendar reads the working calendar of the BV portal and filters the work sessions.
package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/huponx/gocommon/retry"
	"go.uber.org/zap"
)

const calendarURL = "https://api-hcm.banvien.com.vn/gatewayapp/working-hours/calendar/v2"

// VietnamLoc is UTC+7, the timezone of the BV API.
var VietnamLoc = time.FixedZone("UTC+7", 7*60*60)

// vietnamLoc keeps the short name used by filter.go.
var vietnamLoc = VietnamLoc

// DateRange holds the from/to millisecond timestamps of one billing period (21st to 20th).
type DateRange struct {
	FromDate int64
	ToDate   int64
}

// From is the first instant of the period in UTC+7.
func (r DateRange) From() time.Time { return time.UnixMilli(r.FromDate).In(VietnamLoc) }

// To is the last instant of the period in UTC+7.
func (r DateRange) To() time.Time { return time.UnixMilli(r.ToDate).In(VietnamLoc) }

// CurrentPeriod returns the billing period that contains now.
func CurrentPeriod() DateRange { return PeriodAt(time.Now()) }

// PeriodAt returns the billing period that contains t: the 21st of a month to the 20th of the
// next one (UTC+7).
func PeriodAt(t time.Time) DateRange {
	now := t.In(VietnamLoc)
	if now.Day() >= 21 {
		next := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, VietnamLoc)
		return PeriodEndingIn(next.Year(), next.Month())
	}
	return PeriodEndingIn(now.Year(), now.Month())
}

// PeriodEndingIn returns the period that ends on the 20th of the given month, named after that
// month: PeriodEndingIn(2025, time.October) is 21/09/2025 to 20/10/2025.
func PeriodEndingIn(year int, month time.Month) DateRange {
	from := time.Date(year, month-1, 21, 0, 0, 0, 0, VietnamLoc)
	to := time.Date(year, month, 20, 23, 59, 59, 999_000_000, VietnamLoc)
	return DateRange{FromDate: from.UnixMilli(), ToDate: to.UnixMilli()}
}

// GetCalendar fetches the calendar of the user for the date range.
func GetCalendar(ctx context.Context, sessionToken, userID string, period DateRange) ([]Item, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, calendarURL, nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("from_date", strconv.FormatInt(period.FromDate, 10))
	q.Set("to_date", strconv.FormatInt(period.ToDate, 10))
	q.Set("user_id", userID)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Authorization", "Bearer "+sessionToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calendar request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode calendar response: %w", err)
	}
	if result.Code != "CODE_SUCCESS" {
		return nil, fmt.Errorf("calendar returned code %q", result.Code)
	}
	return result.Data.Items, nil
}

// Fetch is GetCalendar with the default retry policy.
func Fetch(ctx context.Context, log *zap.Logger, sessionToken, userID string, period DateRange) ([]Item, error) {
	log.Info("fetching calendar",
		zap.String("user_id", userID),
		zap.Time("from", period.From()),
		zap.Time("to", period.To()),
	)
	var items []Item
	err := retry.Do(ctx, retry.Default, func() error {
		var err error
		items, err = GetCalendar(ctx, sessionToken, userID, period)
		if err != nil {
			log.Warn("calendar request failed, retrying", zap.Error(err))
		}
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("calendar: %w", err)
	}
	return items, nil
}
