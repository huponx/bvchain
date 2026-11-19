// Command bvchain is the check-in/check-out job: it logs in to the BV portal and checks in
// and out the work sessions of today. It runs as a Kubernetes CronJob.
package main

import (
	"fmt"
	"os"

	"github.com/huponx/bvchain/internal/appconfig"
	"github.com/huponx/bvchain/internal/checkin"
	"github.com/huponx/bvchain/internal/notify"
	"github.com/huponx/gocommon/config"
	"github.com/huponx/gocommon/logging"
	"github.com/huponx/gocommon/worker"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load[appconfig.Chain]()
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid config:", err)
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "invalid config:", err)
		os.Exit(1)
	}
	logCfg := appconfig.WithService(cfg.Log, "bvchain")
	log, err := logging.New(logging.Config{Level: logCfg.Level, Encoding: logCfg.Encoding, Service: logCfg.Service, Env: logCfg.Env})
	if err != nil {
		fmt.Fprintln(os.Stderr, "logger:", err)
		os.Exit(1)
	}
	defer logging.Sync(log)

	ctx, stop := worker.Signals()
	defer stop()

	sender, closeConn, err := notify.Dial(ctx, cfg.Courier, log)
	if err != nil {
		log.Error("courier", zap.Error(err))
		logging.Sync(log)
		os.Exit(1)
	}
	defer func() { _ = closeConn() }()

	if err := checkin.New(cfg, log, sender).Run(ctx); err != nil {
		log.Error("job failed", zap.Error(err))
		logging.Sync(log)
		os.Exit(1)
	}
	log.Info("job completed successfully")
}
