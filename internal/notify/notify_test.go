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
	got      []*courierv1.NotifyRequest
	template []*courierv1.SendNotificationRequest
	skipped  []string
	err      error
}

func (f *fakeClient) Notify(_ context.Context, in *courierv1.NotifyRequest, _ ...grpc.CallOption) (*courierv1.NotifyResponse, error) {
	f.got = append(f.got, in)
	return &courierv1.NotifyResponse{SkippedUserIds: f.skipped}, f.err
}

func (f *fakeClient) SendNotification(_ context.Context, in *courierv1.SendNotificationRequest, _ ...grpc.CallOption) (*courierv1.SendNotificationResponse, error) {
	f.template = append(f.template, in)
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
	if req.GetEventType() != "bvchain.checkin" || req.GetLocaleOverride() != "en" || len(req.GetUserIds()) != 0 {
		t.Fatalf("%v", req)
	}
	if d := req.GetData().AsMap(); d["shift"] != "1" || d["time"] != "08:31" {
		t.Fatalf("data: %v", d)
	}
	if req.GetIdempotencyKey() != "bvchain-checkin-2026-10-05-1" {
		t.Fatalf("key: %q", req.GetIdempotencyKey())
	}
	rec := req.GetRecipients()
	if len(rec) != 1 || rec[0].GetChannel() != courierv1.Channel_CHANNEL_TELEGRAM || rec[0].GetAddress() != "-100123" {
		t.Fatalf("recipients: %v", rec)
	}

	if err := CheckOut(context.Background(), c, 2, at); err != nil || f.got[1].GetEventType() != "bvchain.checkout" {
		t.Fatalf("checkout: %v", err)
	}
	if err := Failed(context.Background(), c, "calendar", errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	if d := f.got[2].GetData().AsMap(); f.got[2].GetEventType() != "bvchain.failed" || d["step"] != "calendar" || d["error"] != "boom" {
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

// A configured user is reached by id, in the chat they linked; there is no raw chat in the request.
func TestCourierSendsToTheLinkedUser(t *testing.T) {
	f := &fakeClient{}
	c := New(f, "-100123", "vi").ForUser("user-1")
	if err := CheckIn(context.Background(), c, 2, time.Date(2026, 10, 5, 13, 30, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	req := f.got[0]
	if len(req.GetUserIds()) != 1 || req.GetUserIds()[0] != "user-1" || len(req.GetRecipients()) != 0 {
		t.Fatalf("request: %v", req)
	}
}

// A user who has not linked Telegram is skipped by courier: a log line, not a failed job.
func TestSkippedUserIsNotAnError(t *testing.T) {
	f := &fakeClient{skipped: []string{"user-1"}}
	c := New(f, "", "vi").ForUser("user-1")
	if err := c.Send(context.Background(), TemplateFailed, "", map[string]string{"step": "s", "error": "e"}); err != nil {
		t.Fatal(err)
	}
}

// A template courier has no event type for keeps the older call.
func TestUnknownTemplateKeepsSendNotification(t *testing.T) {
	f := &fakeClient{}
	c := New(f, "-1", "en")
	if err := c.Send(context.Background(), "custom", "", map[string]string{"a": "b"}); err != nil {
		t.Fatal(err)
	}
	if len(f.got) != 0 || len(f.template) != 1 || f.template[0].GetTemplateId() != "custom" {
		t.Fatalf("notify %v template %v", f.got, f.template)
	}
}
