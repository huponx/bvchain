// Package appconfig holds the environment configuration of the jobs and bvctl.
package appconfig

import (
	"fmt"
	"strings"
	"time"

	"github.com/huponx/gocommon/config"
)

// BV is the account of the BV portal.
type BV struct {
	Username  string `env:"BV_USERNAME"`
	Password  string `env:"BV_PASSWORD"`
	OTPSecret string `env:"BV_OTP_SECRET"`
}

// Courier is the courier gRPC API that delivers the Telegram messages.
type Courier struct {
	// Target is host:port of courier (gRPC).
	Target  string        `env:"COURIER_TARGET"`
	Timeout time.Duration `env:"COURIER_TIMEOUT" envDefault:"10s"`
	// Insecure is plaintext gRPC, the in-cluster default (courier has no TLS of its own).
	Insecure bool `env:"COURIER_INSECURE" envDefault:"true"`
	// TokenFile is the projected ServiceAccount token (audience courier); it wins over S2SToken.
	TokenFile string `env:"COURIER_TOKEN_FILE"`
	// S2SToken is a static Bearer token for a local courier started with COURIER_S2S_TOKEN.
	S2SToken string `env:"COURIER_S2S_TOKEN"`
	// ChatID is the Telegram chat that receives the messages.
	ChatID string `env:"TELEGRAM_CHAT_ID"`
	// Locale selects the template language (vi, en).
	Locale string `env:"NOTIFY_LOCALE" envDefault:"vi"`
}

// Chain is the configuration of the check-in/check-out job (cmd/bvchain).
type Chain struct {
	config.Log
	BV      BV
	Courier Courier
	// Delay is the upper bound of a random sleep before the run, so the check-in time varies.
	Delay time.Duration `env:"DELAY" envDefault:"0s"`
	// SuccessNotify sends a Telegram message after a successful check-in/check-out.
	SuccessNotify bool `env:"NOTIFICATION_SUCCESS_ENABLED" envDefault:"false"`
}

// Report is the configuration of the working-days report job (cmd/bvreport).
type Report struct {
	config.Log
	BV      BV
	Courier Courier
	// AlwaysSend sends the report even when no day is missing.
	AlwaysSend bool `env:"REPORT_ALWAYS_SEND" envDefault:"false"`
}

// Ctl is the configuration of bvctl. Which of BV and Courier is needed depends on the command,
// so nothing is required here: the command calls the Validate it needs.
type Ctl struct {
	config.Log
	BV      BV
	Courier Courier
}

// Validate checks that the account of the BV portal is complete.
func (b BV) Validate() error {
	var missing []string
	if strings.TrimSpace(b.Username) == "" {
		missing = append(missing, "BV_USERNAME")
	}
	if strings.TrimSpace(b.Password) == "" {
		missing = append(missing, "BV_PASSWORD")
	}
	if strings.TrimSpace(b.OTPSecret) == "" {
		missing = append(missing, "BV_OTP_SECRET")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing: %s", strings.Join(missing, ", "))
	}
	return nil
}

// Validate checks that courier and the chat are set.
func (c Courier) Validate() error {
	var missing []string
	if strings.TrimSpace(c.Target) == "" {
		missing = append(missing, "COURIER_TARGET")
	}
	if strings.TrimSpace(c.ChatID) == "" {
		missing = append(missing, "TELEGRAM_CHAT_ID")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing: %s", strings.Join(missing, ", "))
	}
	return nil
}

// Validate checks the BV account and courier settings.
func (c Chain) Validate() error { return validate(c.BV, c.Courier) }

// Validate checks the BV account and courier settings.
func (c Report) Validate() error { return validate(c.BV, c.Courier) }

func validate(b BV, c Courier) error {
	if err := b.Validate(); err != nil {
		return err
	}
	return c.Validate()
}

// WithService sets the logger service name when SERVICE_NAME was not given.
func WithService(l config.Log, name string) config.Log {
	if l.Service == "" || l.Service == "app" {
		l.Service = name
	}
	return l
}
