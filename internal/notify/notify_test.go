package notify

import (
	"context"
	"errors"
	"testing"
	"time"

	courierv1 "github.com/huponx/protogen/go/courier/v1"
	"google.golang.org/grpc"
)

type fakeClient struct {
	courierv1.CourierServiceClient
	got []*courierv1.SendNotificationRequest
	err error
}

func (f *fakeClient) SendNotification(_ context.Context, in *courierv1.SendNotificationRequest, _ ...grpc.CallOption) (*courierv1.SendNotificationResponse, error) {
	f.got = append(f.got, in)
	return &courierv1.SendNotificationResponse{}, f.err
}

func TestCourierSendsTelegramToTheChat(t *testing.T) {
	f := &fakeClient{}
	c := New(f, "-100123", "en")
	at := time.Date(2026, 10, 5, 8, 31, 0, 0, time.UTC)
	if err := CheckIn(context.Background(), c, 1, at); err != nil {
		t.Fatal(err)
	}
	req := f.got[0]
	if req.GetTemplateId() != TemplateCheckIn || req.GetLocale() != "en" {
		t.Fatalf("%v", req)
	}
	if req.GetTemplateData()["shift"] != "1" || req.GetTemplateData()["time"] != "08:31" {
		t.Fatalf("data: %v", req.GetTemplateData())
	}
	if req.GetIdempotencyKey() != "bvchain-checkin-2026-10-05-1" {
		t.Fatalf("key: %q", req.GetIdempotencyKey())
	}
	rec := req.GetRecipients()
	if len(rec) != 1 || rec[0].GetChannel() != courierv1.Channel_CHANNEL_TELEGRAM || rec[0].GetAddress() != "-100123" {
		t.Fatalf("recipients: %v", rec)
	}

	if err := CheckOut(context.Background(), c, 2, at); err != nil || f.got[1].GetTemplateId() != TemplateCheckOut {
		t.Fatalf("checkout: %v", err)
	}
	if err := Failed(context.Background(), c, "calendar", errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	if d := f.got[2].GetTemplateData(); f.got[2].GetTemplateId() != TemplateFailed || d["step"] != "calendar" || d["error"] != "boom" {
		t.Fatalf("failed: %v", f.got[2])
	}
}

func TestSendWrapsTheError(t *testing.T) {
	c := New(&fakeClient{err: errors.New("unavailable")}, "1", "")
	err := c.Send(context.Background(), TemplateReport, "", nil)
	if err == nil || c.locale != "vi" {
		t.Fatalf("err = %v, locale = %q", err, c.locale)
	}
}
