// Package checkin is the check-in/check-out job (cmd/bvchain).
package checkin

import (
	"context"
	"fmt"
	"time"

	"github.com/huponx/bvchain/internal/appconfig"
	"github.com/huponx/bvchain/internal/auth"
	"github.com/huponx/bvchain/internal/calendar"
	"github.com/huponx/bvchain/internal/cico"
	"github.com/huponx/bvchain/internal/delay"
	"github.com/huponx/bvchain/internal/notify"
	"github.com/huponx/gocommon/retry"
	"go.uber.org/zap"
)

// Job checks in and out the work sessions of today.
type Job struct {
	cfg    appconfig.Chain
	log    *zap.Logger
	sender notify.Sender
}

// New creates the job. sender delivers the Telegram messages.
func New(cfg appconfig.Chain, log *zap.Logger, sender notify.Sender) *Job {
	return &Job{cfg: cfg, log: log, sender: sender}
}

// Run executes the job and returns an error when a step failed after its retries.
func (j *Job) Run(ctx context.Context) error {
	j.log.Info("starting bvchain job")

	if err := delay.Random(ctx, j.cfg.Delay, j.log); err != nil {
		return err
	}

	sess, err := auth.GetSession(ctx, auth.Credentials{
		Username:  j.cfg.BV.Username,
		Password:  j.cfg.BV.Password,
		OTPSecret: j.cfg.BV.OTPSecret,
	}, j.log)
	if err != nil {
		j.fail(ctx, "authentication", err)
		return fmt.Errorf("get session: %w", err)
	}

	items, err := calendar.Fetch(ctx, j.log, sess.Token, sess.UserID, calendar.CurrentPeriod())
	if err != nil {
		j.fail(ctx, "calendar", err)
		return err
	}

	sessions := calendar.FilterWorkSessions(items)
	j.log.Info("working sessions retrieved", zap.Int("count", len(sessions)))

	today := calendar.CurrentSessions(sessions)
	j.log.Info("current working sessions", zap.Int("count", len(today)))

	for _, s := range today {
		if err := j.handle(ctx, sess, s); err != nil {
			return err
		}
	}
	j.log.Info("all sessions processed")
	return nil
}

// handle checks in when not yet checked in, and checks out once the shift is over.
func (j *Job) handle(ctx context.Context, sess auth.Session, s calendar.WorkSession) error {
	log := j.log.With(zap.String("schedule_id", s.ScheduleID), zap.Int("shift", s.WorkShift))

	// time_in == 0: not checked in yet.
	if s.TimeIn == 0 {
		log.Info("checking in")
		if err := j.record(ctx, sess, "check-in"); err != nil {
			j.fail(ctx, "check-in", err)
			return fmt.Errorf("check-in shift %d: %w", s.WorkShift, err)
		}
		if j.cfg.SuccessNotify {
			j.notified(ctx, "check-in", notify.CheckIn(ctx, j.sender, s.WorkShift, time.Now().In(calendar.VietnamLoc)))
		}
	} else {
		log.Info("already checked in, skipping")
	}

	// Morning (shift 1) checks out from 12:00, afternoon (shift 2) from 17:30.
	// AlreadyCheckedOut covers a delayed run that arrives after an earlier or manual checkout.
	switch {
	case s.AlreadyCheckedOut:
		log.Info("already checked out, skipping")
	case !calendar.ShouldCheckOut(s.WorkShift):
		log.Info("not yet time to check out, skipping")
	default:
		log.Info("checking out")
		if err := j.record(ctx, sess, "check-out"); err != nil {
			j.fail(ctx, "check-out", err)
			return fmt.Errorf("check-out shift %d: %w", s.WorkShift, err)
		}
		if j.cfg.SuccessNotify {
			j.notified(ctx, "check-out", notify.CheckOut(ctx, j.sender, s.WorkShift, time.Now().In(calendar.VietnamLoc)))
		}
	}
	return nil
}

func (j *Job) record(ctx context.Context, sess auth.Session, what string) error {
	return retry.Do(ctx, retry.Default, func() error {
		err := cico.Record(ctx, sess.Token)
		if err != nil {
			j.log.Warn(what+" failed, retrying", zap.Error(err))
		}
		return err
	})
}

// fail tells the chat that a step failed; a failed message is only logged.
func (j *Job) fail(ctx context.Context, step string, cause error) {
	j.notified(ctx, "failure message", notify.Failed(ctx, j.sender, step, cause))
}

func (j *Job) notified(_ context.Context, what string, err error) {
	if err != nil {
		j.log.Warn("notification failed", zap.String("what", what), zap.Error(err))
	}
}
