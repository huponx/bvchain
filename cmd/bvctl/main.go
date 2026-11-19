// Command bvctl is the manual tool of bvchain: it sends the Telegram messages of the jobs
// with sample data (to test the templates and the courier wiring) and builds the real working
// report of a month. See the README, section "bvctl".
//
//	bvctl notify checkin|checkout|failed|report [-chat ID] [-locale vi|en] [-data k=v]
//	bvctl report [-month MM/YYYY] [-chat ID] [-locale vi|en] [-dry-run] [-verbose] [-skip-empty] [-no-dedupe]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/huponx/bvchain/internal/appconfig"
	"github.com/huponx/bvchain/internal/auth"
	"github.com/huponx/bvchain/internal/calendar"
	"github.com/huponx/bvchain/internal/notify"
	"github.com/huponx/bvchain/internal/report"
	"github.com/huponx/gocommon/config"
	"github.com/huponx/gocommon/logging"
	"github.com/huponx/gocommon/worker"
	"go.uber.org/zap"
)

const usage = `usage:
  bvctl notify <checkin|checkout|failed|report> [-chat ID] [-locale vi|en] [-data key=value]...
      Send a message of the jobs with sample data. No login to BV.
      Needs COURIER_TARGET (+ COURIER_TOKEN_FILE or COURIER_S2S_TOKEN) and TELEGRAM_CHAT_ID (or -chat).
  bvctl report [-month MM/YYYY] [-chat ID] [-locale vi|en] [-dry-run] [-verbose] [-skip-empty] [-no-dedupe]
      Log in to BV, count the working days of the period that ends in MM/YYYY (default: the
      current period) and send the report. -dry-run prints it instead of sending.
      Like the job, the same report is sent once a day; -no-dedupe sends it again.
      Needs BV_USERNAME, BV_PASSWORD, BV_OTP_SECRET and, to send, the courier variables.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "notify":
		err = runNotify(os.Args[2:])
	case "report":
		err = runReport(os.Args[2:])
	case "-h", "-help", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		var u usageError
		if errors.As(err, &u) {
			fmt.Fprintln(os.Stderr, "bvctl:", err)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "bvctl:", err)
		os.Exit(1)
	}
}

type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

type kvMap map[string]string

func (m kvMap) String() string { return "" }

func (m kvMap) Set(v string) error {
	k, val, ok := strings.Cut(v, "=")
	if !ok || strings.TrimSpace(k) == "" {
		return fmt.Errorf("data must be key=value")
	}
	m[strings.TrimSpace(k)] = val
	return nil
}

func setup(verbose bool) (appconfig.Ctl, *zap.Logger, error) {
	cfg, err := config.Load[appconfig.Ctl]()
	if err != nil {
		return cfg, nil, fmt.Errorf("invalid config: %w", err)
	}
	logCfg := appconfig.WithService(cfg.Log, "bvctl")
	if verbose {
		logCfg.Level = "debug"
	}
	log, err := logging.New(logging.Config{Level: logCfg.Level, Encoding: logCfg.Encoding, Service: logCfg.Service, Env: logCfg.Env})
	return cfg, log, err
}

func runNotify(args []string) error {
	fs := flag.NewFlagSet("notify", flag.ContinueOnError)
	chat := fs.String("chat", "", "Telegram chat id (default TELEGRAM_CHAT_ID)")
	locale := fs.String("locale", "", "template locale vi or en (default NOTIFY_LOCALE, vi)")
	shift := fs.Int("shift", 1, "shift of checkin/checkout: 1 morning, 2 afternoon")
	at := fs.String("time", "", "time shown by checkin/checkout, HH:MM (default now)")
	data := kvMap{}
	fs.Var(data, "data", "override a template data key=value (repeatable)")
	if len(args) == 0 {
		return usageError{"notify needs checkin, checkout, failed or report"}
	}
	kind := args[0]
	if err := fs.Parse(args[1:]); err != nil {
		return usageError{err.Error()}
	}

	cfg, log, err := setup(false)
	if err != nil {
		return err
	}
	defer logging.Sync(log)
	if *chat != "" {
		cfg.Courier.ChatID = *chat
	}
	if *locale != "" {
		cfg.Courier.Locale = *locale
	}

	templateID, tplData, err := sample(kind, *shift, *at, time.Now().In(calendar.VietnamLoc))
	if err != nil {
		return usageError{err.Error()}
	}
	for k, v := range data {
		tplData[k] = v
	}

	ctx, stop := worker.Signals()
	defer stop()
	sender, closeConn, err := notify.Dial(ctx, cfg.Courier, log)
	if err != nil {
		return err
	}
	defer func() { _ = closeConn() }()

	// No idempotency key: every test run sends a message.
	if err := sender.Send(ctx, templateID, "", tplData); err != nil {
		return err
	}
	fmt.Printf("sent %s (locale %s) to chat %s\n", templateID, cfg.Courier.Locale, cfg.Courier.ChatID)
	return nil
}

// sample returns the template id and the sample data of a message kind.
func sample(kind string, shift int, at string, now time.Time) (string, map[string]string, error) {
	switch kind {
	case "checkin", "checkout":
		if shift != 1 && shift != 2 {
			return "", nil, fmt.Errorf("-shift must be 1 or 2, got %d", shift)
		}
		clock := now
		if at != "" {
			t, err := time.Parse("15:04", at)
			if err != nil {
				return "", nil, fmt.Errorf("-time must be HH:MM, got %q", at)
			}
			clock = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location())
		}
		id := notify.TemplateCheckIn
		if kind == "checkout" {
			id = notify.TemplateCheckOut
		}
		return id, notify.ShiftData(shift, clock), nil
	case "failed":
		return notify.TemplateFailed, map[string]string{"step": "authentication", "error": "sample error from bvctl"}, nil
	case "report":
		return notify.TemplateReport, map[string]string{
			"from_date": "21/09", "to_date": "20/10", "today": "19/10",
			"total_in_period": "22", "total_to_today": "21", "days_with_cico": "20", "missing_days": "01",
		}, nil
	}
	return "", nil, fmt.Errorf("unknown message %q (checkin, checkout, failed or report)", kind)
}

func runReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	month := fs.String("month", "", "report period named after the month it ends in, MM/YYYY (10/2025 is 21/09 to 20/10); default the current period")
	chat := fs.String("chat", "", "Telegram chat id (default TELEGRAM_CHAT_ID)")
	locale := fs.String("locale", "", "template locale vi or en (default NOTIFY_LOCALE, vi)")
	dry := fs.Bool("dry-run", false, "print the report, send nothing")
	verbose := fs.Bool("verbose", false, "debug log and a per-day table")
	skipEmpty := fs.Bool("skip-empty", false, "do not send when no day is missing (the job's behaviour)")
	noDedupe := fs.Bool("no-dedupe", false, "send again even when the same report was already sent today (courier ignores a repeated key for 24h)")
	if err := fs.Parse(args); err != nil {
		return usageError{err.Error()}
	}

	now := time.Now()
	period := calendar.PeriodAt(now)
	if *month != "" {
		p, err := ParseMonth(*month)
		if err != nil {
			return usageError{err.Error()}
		}
		period = p
	}

	cfg, log, err := setup(*verbose)
	if err != nil {
		return err
	}
	defer logging.Sync(log)
	if *chat != "" {
		cfg.Courier.ChatID = *chat
	}
	if *locale != "" {
		cfg.Courier.Locale = *locale
	}
	if err := cfg.BV.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	ctx, stop := worker.Signals()
	defer stop()

	var sender notify.Sender = refuseSender{}
	if !*dry {
		s, closeConn, err := notify.Dial(ctx, cfg.Courier, log)
		if err != nil {
			return err
		}
		defer func() { _ = closeConn() }()
		sender = s
	}

	r := &report.Reporter{
		Creds:  auth.Credentials{Username: cfg.BV.Username, Password: cfg.BV.Password, OTPSecret: cfg.BV.OTPSecret},
		Log:    log,
		Sender: sender,
	}
	dedupe := report.DedupeDaily
	if *noDedupe {
		dedupe = report.DedupeNone
	}
	res, err := r.Run(ctx, period, now, report.Options{DryRun: *dry, SendAlways: !*skipEmpty, Dedupe: dedupe})
	if err != nil {
		return err
	}

	fmt.Printf("period %s to %s\n", period.From().Format("2006-01-02"), period.To().Format("2006-01-02"))
	if *verbose {
		fmt.Print(res.Summary.Table())
	}
	keys := make([]string, 0, len(res.Data))
	for k := range res.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%-16s %s\n", k, res.Data[k])
	}
	fmt.Printf("legacy count (old report): %d, working days now: %s\n", res.Summary.LegacyTotalInPeriod, res.Data["total_in_period"])
	switch {
	case *dry:
		fmt.Println("dry run: nothing sent")
	case res.Sent:
		fmt.Printf("report sent to chat %s\n", cfg.Courier.ChatID)
	default:
		fmt.Println("nothing sent (no missing days)")
	}
	return nil
}

// ParseMonth parses MM/YYYY into the period that ends on the 20th of that month.
func ParseMonth(s string) (calendar.DateRange, error) {
	t, err := time.Parse("01/2006", strings.TrimSpace(s))
	if err != nil {
		return calendar.DateRange{}, fmt.Errorf("-month must be MM/YYYY, got %q", s)
	}
	return calendar.PeriodEndingIn(t.Year(), t.Month()), nil
}

// refuseSender is used by -dry-run: sending is never reached, and a bug that did would fail loudly.
type refuseSender struct{}

func (refuseSender) Send(context.Context, string, string, map[string]string) error {
	return errors.New("dry run: sending is disabled")
}
