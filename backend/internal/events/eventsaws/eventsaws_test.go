package eventsaws

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/scheduler"
	schedtypes "github.com/aws/aws-sdk-go-v2/service/scheduler/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"

	"gp/internal/events"
)

type fakeSQS struct {
	in  *sqs.SendMessageBatchInput
	out *sqs.SendMessageBatchOutput
	err error
}

func (f *fakeSQS) SendMessageBatch(_ context.Context, in *sqs.SendMessageBatchInput, _ ...func(*sqs.Options)) (*sqs.SendMessageBatchOutput, error) {
	f.in = in
	return f.out, f.err
}

func TestQueueSend(t *testing.T) {
	api := &fakeSQS{out: &sqs.SendMessageBatchOutput{Failed: []sqstypes.BatchResultErrorEntry{{Id: aws.String("evt_B")}}}}
	q := NewQueue(api, "https://sqs.example/queue")
	failed, err := q.Send(context.Background(), []events.Outgoing{
		{ID: "evt_A", Body: []byte(`{"a":1}`)},
		{ID: "evt_B", Body: []byte(`{"b":2}`), Delay: 120 * time.Second},
	})
	if err != nil || fmt.Sprint(failed) != "[evt_B]" {
		t.Fatalf("failed %v, err %v", failed, err)
	}
	if *api.in.QueueUrl != "https://sqs.example/queue" || len(api.in.Entries) != 2 {
		t.Fatalf("input = %+v", api.in)
	}
	a, b := api.in.Entries[0], api.in.Entries[1]
	if *a.Id != "evt_A" || *a.MessageBody != `{"a":1}` || a.DelaySeconds != 0 || b.DelaySeconds != 120 {
		t.Errorf("entries = %+v / %+v", a, b)
	}

	api.err = errors.New("throttled")
	if _, err := q.Send(context.Background(), []events.Outgoing{{ID: "x"}}); err == nil {
		t.Error("an SQS error must be returned")
	}
}

type fakeScheduler struct {
	in  *scheduler.CreateScheduleInput
	err error
}

func (f *fakeScheduler) CreateSchedule(_ context.Context, in *scheduler.CreateScheduleInput, _ ...func(*scheduler.Options)) (*scheduler.CreateScheduleOutput, error) {
	f.in = in
	return &scheduler.CreateScheduleOutput{}, f.err
}

func TestCreateSchedule(t *testing.T) {
	api := &fakeScheduler{}
	s := NewScheduler(api, "grocery-dev", "arn:aws:lambda:x:1:function:timer", "arn:aws:iam::1:role/scheduler")
	id := uuid.MustParse("3f2b8f0e-0000-4000-8000-00000000abcd")
	tm := events.Timer{OrderID: id, Kind: events.TimerReview, RevisionNo: 1, RunAt: time.Date(2026, 11, 2, 8, 15, 30, 0, time.FixedZone("Colombo", 5*3600+1800))}
	if err := s.CreateSchedule(context.Background(), tm); err != nil {
		t.Fatal(err)
	}
	in := api.in
	if *in.Name != "ord-3f2b8f0e-0000-4000-8000-00000000abcd-review-1" || *in.GroupName != "grocery-dev" {
		t.Errorf("name/group = %s / %s", *in.Name, *in.GroupName)
	}
	if len(*in.Name) > 64 {
		t.Errorf("name is %d long, Scheduler allows 64", len(*in.Name))
	}
	// 08:15:30 at +05:30 is 02:45:30 UTC.
	if *in.ScheduleExpression != "at(2026-11-02T02:45:30)" || *in.ScheduleExpressionTimezone != "UTC" {
		t.Errorf("expression = %s %s", *in.ScheduleExpression, *in.ScheduleExpressionTimezone)
	}
	if in.ActionAfterCompletion != schedtypes.ActionAfterCompletionDelete || in.FlexibleTimeWindow.Mode != schedtypes.FlexibleTimeWindowModeOff {
		t.Errorf("completion %s, window %s", in.ActionAfterCompletion, in.FlexibleTimeWindow.Mode)
	}
	if *in.Target.Arn != "arn:aws:lambda:x:1:function:timer" || *in.Target.RoleArn != "arn:aws:iam::1:role/scheduler" {
		t.Errorf("target = %+v", in.Target)
	}
	if *in.Target.Input != `{"orderId":"3f2b8f0e-0000-4000-8000-00000000abcd","kind":"review","revisionNo":1}` {
		t.Errorf("input = %s", *in.Target.Input)
	}
}

func TestCreateScheduleConflictIsSuccess(t *testing.T) {
	api := &fakeScheduler{err: &schedtypes.ConflictException{Message: aws.String("exists")}}
	s := NewScheduler(api, "g", "arn", "role")
	tm := events.Timer{OrderID: uuid.New(), Kind: events.TimerPayment, RunAt: time.Now().Add(time.Hour)}
	if err := s.CreateSchedule(context.Background(), tm); err != nil {
		t.Errorf("a schedule that exists is success: %v", err)
	}
	api.err = errors.New("access denied")
	if err := s.CreateSchedule(context.Background(), tm); err == nil || !strings.Contains(err.Error(), "create schedule") {
		t.Errorf("another error must be returned: %v", err)
	}
}

func TestScheduleNamesFitTheLimit(t *testing.T) {
	longest := events.Timer{OrderID: uuid.New(), Kind: events.TimerRevision, RevisionNo: 2147483647}
	if n := len(longest.Name()); n > 64 {
		t.Errorf("name is %d long, limit 64", n)
	}
}
