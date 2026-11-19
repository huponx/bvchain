package report

import (
	"context"
	"fmt"
	"time"

	"github.com/huponx/bvchain/internal/appconfig"
	"github.com/huponx/bvchain/internal/auth"
	"github.com/huponx/bvchain/internal/calendar"
	"github.com/huponx/bvchain/internal/delay"
	"github.com/huponx/bvchain/internal/notify"
	"go.uber.org/zap"
)

// Reporter builds the working report of a period and sends it.
type Reporter struct {
	Creds  auth.Credentials
	Log    *zap.Logger
	Sender notify.Sender
}

// Options tune one run.
type Options struct {
	// DryRun computes the report but sends nothing.
	DryRun bool
	// SendAlways sends the report even when no day is missing.
	SendAlways bool
}

// Result is the outcome of a run.
type Result struct {
	Period  calendar.DateRange
	Summary Summary
	Data    map[string]string
	Sent    bool
}

// Run logs in, reads the calendar of the period, counts the working days and sends the
// report (when a day is missing or SendAlways is set, and not DryRun). `now` decides which
// days are "up to today".
func (r *Reporter) Run(ctx context.Context, period calendar.DateRange, now time.Time, opts Options) (Result, error) {
	res := Result{Period: period}

	sess, err := auth.GetSession(ctx, r.Creds, r.Log)
	if err != nil {
		if !opts.DryRun {
			r.fail(ctx, "authentication", err)
		}
		return res, fmt.Errorf("get session: %w", err)
	}

	items, err := calendar.Fetch(ctx, r.Log, sess.Token, sess.UserID, period)
	if err != nil {
		if !opts.DryRun {
			r.fail(ctx, "calendar", err)
		}
		return res, err
	}

	res.Summary = Summarize(items, period, now, r.Log)
	res.Data = res.Summary.Data(period, now)

	switch {
	case opts.DryRun:
		r.Log.Info("dry run: report not sent")
	case res.Summary.MissingDays <= 0 && !opts.SendAlways:
		r.Log.Info("no missing days, skipping report")
	default:
		key := fmt.Sprintf("bvreport-%s-%s", calendar.DayKey(period.FromDate), now.In(calendar.VietnamLoc).Format("2006-01-02"))
		if err := r.Sender.Send(ctx, notify.TemplateReport, key, res.Data); err != nil {
			return res, fmt.Errorf("send report: %w", err)
		}
		res.Sent = true
		r.Log.Info("report sent")
	}
	return res, nil
}

func (r *Reporter) fail(ctx context.Context, step string, cause error) {
	if err := notify.Failed(ctx, r.Sender, step, cause); err != nil {
		r.Log.Warn("failure message not sent", zap.Error(err))
	}
}

// Job is the scheduled report (cmd/bvreport): the current period, up to now.
type Job struct {
	cfg      appconfig.Report
	reporter *Reporter
}

// NewJob creates the scheduled report job.
func NewJob(cfg appconfig.Report, log *zap.Logger, sender notify.Sender) *Job {
	return &Job{cfg: cfg, reporter: &Reporter{
		Creds:  auth.Credentials{Username: cfg.BV.Username, Password: cfg.BV.Password, OTPSecret: cfg.BV.OTPSecret},
		Log:    log,
		Sender: sender,
	}}
}

// Run executes the job.
func (j *Job) Run(ctx context.Context) error {
	j.reporter.Log.Info("starting bvreport job")
	if err := delay.Random(ctx, j.cfg.Delay, j.reporter.Log); err != nil {
		return err
	}
	now := time.Now()
	_, err := j.reporter.Run(ctx, calendar.PeriodAt(now), now, Options{SendAlways: j.cfg.AlwaysSend})
	return err
}
