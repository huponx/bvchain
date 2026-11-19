package appconfig

import (
	"testing"
	"time"

	"github.com/huponx/gocommon/config"
)

func TestChainFromEnv(t *testing.T) {
	t.Setenv("BV_USERNAME", "hung.pham")
	t.Setenv("BV_PASSWORD", "pw")
	t.Setenv("BV_OTP_SECRET", "GEZDGNBV")
	t.Setenv("COURIER_TARGET", "courier.application.svc.cluster.local:9090")
	t.Setenv("TELEGRAM_CHAT_ID", "-100123")
	t.Setenv("DELAY", "3m")
	cfg, err := config.Load[Chain]()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.Delay != 3*time.Minute || cfg.Courier.Locale != "vi" || !cfg.Courier.Insecure || cfg.Courier.Timeout != 10*time.Second || cfg.SuccessNotify {
		t.Fatalf("%+v", cfg)
	}
}

func TestValidateListsWhatIsMissing(t *testing.T) {
	t.Setenv("COURIER_TARGET", "x:1")
	cfg, err := config.Load[Report]()
	if err != nil {
		t.Fatal(err)
	}
	err = cfg.Validate()
	if err == nil || err.Error() != "missing: BV_USERNAME, BV_PASSWORD, BV_OTP_SECRET" {
		t.Fatalf("err = %v", err)
	}
	if err := (Courier{Target: "x:1"}).Validate(); err == nil || err.Error() != "missing: TELEGRAM_CHAT_ID" {
		t.Fatalf("err = %v", err)
	}
}
