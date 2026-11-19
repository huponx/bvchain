// Package notify sends the Telegram messages of bvchain and bvreport through courier. The
// templates (bvchain_checkin, bvchain_checkout, bvchain_failed, bvreport_working_report) live
// in the courier repository, internal/notify/templates/{locale}/{id}/telegram.txt.tmpl.
package notify

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/huponx/bvchain/internal/appconfig"
	"github.com/huponx/gocommon/grpcclient"
	courierv1 "github.com/huponx/protogen/go/courier/v1"
	"go.uber.org/zap"
)

// Template ids (courier).
const (
	TemplateCheckIn  = "bvchain_checkin"
	TemplateCheckOut = "bvchain_checkout"
	TemplateFailed   = "bvchain_failed"
	TemplateReport   = "bvreport_working_report"
)

// Sender delivers one templated message to the configured chat.
type Sender interface {
	// Send renders templateID with data and delivers it. An empty key means no idempotency key.
	Send(ctx context.Context, templateID, idempotencyKey string, data map[string]string) error
}

// Courier is a Sender backed by the courier gRPC API.
type Courier struct {
	client courierv1.CourierServiceClient
	chatID string
	locale string
	log    *zap.Logger
}

// New wraps a courier client. chatID is the Telegram chat, locale the template language.
func New(client courierv1.CourierServiceClient, chatID, locale string) *Courier {
	if locale == "" {
		locale = "vi"
	}
	return &Courier{client: client, chatID: chatID, locale: locale}
}

// Dial connects to courier (the ServiceAccount token file is re-read when the kubelet rotates
// it) and returns the sender and a close function.
func Dial(ctx context.Context, cfg appconfig.Courier, log *zap.Logger) (*Courier, func() error, error) {
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}
	opts := []grpcclient.Option{
		grpcclient.WithLogger(log),
		grpcclient.WithTimeout(cfg.Timeout),
		grpcclient.WithInsecure(cfg.Insecure),
	}
	if cfg.TokenFile != "" {
		opts = append(opts, grpcclient.WithBearerTokenFile(cfg.TokenFile))
	} else if cfg.S2SToken != "" {
		opts = append(opts, grpcclient.WithBearerToken(cfg.S2SToken))
	}
	conn, err := grpcclient.Dial(ctx, cfg.Target, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("dial courier: %w", err)
	}
	c := New(courierv1.NewCourierServiceClient(conn), cfg.ChatID, cfg.Locale)
	c.log = log
	return c, conn.Close, nil
}

// Send implements Sender.
func (c *Courier) Send(ctx context.Context, templateID, key string, data map[string]string) error {
	resp, err := c.client.SendNotification(ctx, &courierv1.SendNotificationRequest{
		IdempotencyKey: key,
		Locale:         c.locale,
		TemplateId:     templateID,
		TemplateData:   data,
		Recipients: []*courierv1.Recipient{{
			Channel: courierv1.Channel_CHANNEL_TELEGRAM,
			Address: c.chatID,
		}},
	})
	if err != nil {
		return fmt.Errorf("courier %s: %w", templateID, err)
	}
	// queued == 0 means courier accepted the request but queued nothing (duplicate idempotency key)
	if c.log != nil {
		c.log.Info("notification accepted by courier", zap.String("template", templateID),
			zap.String("notification_id", resp.GetNotificationId()), zap.Int32("queued", resp.GetQueued()))
	}
	return nil
}

// CheckIn announces a check-in of the shift (1 morning, 2 afternoon) done at `at`.
func CheckIn(ctx context.Context, s Sender, shift int, at time.Time) error {
	return s.Send(ctx, TemplateCheckIn, shiftKey("bvchain-checkin", shift, at), ShiftData(shift, at))
}

// CheckOut announces a check-out of the shift done at `at`.
func CheckOut(ctx context.Context, s Sender, shift int, at time.Time) error {
	return s.Send(ctx, TemplateCheckOut, shiftKey("bvchain-checkout", shift, at), ShiftData(shift, at))
}

// Failed reports a failed step (for example "authentication") with its error.
func Failed(ctx context.Context, s Sender, step string, cause error) error {
	return s.Send(ctx, TemplateFailed, "", map[string]string{"step": step, "error": cause.Error()})
}

// ShiftData is the template data of a check-in or check-out message.
func ShiftData(shift int, at time.Time) map[string]string {
	return map[string]string{"shift": strconv.Itoa(shift), "time": at.Format("15:04")}
}

// shiftKey makes a retried run of the same shift and day send the message once.
func shiftKey(prefix string, shift int, at time.Time) string {
	return fmt.Sprintf("%s-%s-%d", prefix, at.Format("2006-01-02"), shift)
}
