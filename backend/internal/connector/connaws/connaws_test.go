package connaws

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/scheduler"
	schedtypes "github.com/aws/aws-sdk-go-v2/service/scheduler/types"
	"github.com/google/uuid"

	"gp/internal/connector"
	"gp/internal/shared/enum"
)

type fakeScheduler struct {
	created []*scheduler.CreateScheduleInput
	updates []*scheduler.UpdateScheduleInput
	exists  bool
	deleted []string
	missing bool
}

func (f *fakeScheduler) CreateSchedule(_ context.Context, in *scheduler.CreateScheduleInput, _ ...func(*scheduler.Options)) (*scheduler.CreateScheduleOutput, error) {
	if f.exists {
		return nil, &schedtypes.ConflictException{}
	}
	f.created = append(f.created, in)
	f.exists = true
	return &scheduler.CreateScheduleOutput{}, nil
}

func (f *fakeScheduler) UpdateSchedule(_ context.Context, in *scheduler.UpdateScheduleInput, _ ...func(*scheduler.Options)) (*scheduler.UpdateScheduleOutput, error) {
	f.updates = append(f.updates, in)
	return &scheduler.UpdateScheduleOutput{}, nil
}

func (f *fakeScheduler) DeleteSchedule(_ context.Context, in *scheduler.DeleteScheduleInput, _ ...func(*scheduler.Options)) (*scheduler.DeleteScheduleOutput, error) {
	f.deleted = append(f.deleted, *in.Name)
	if f.missing {
		return nil, &schedtypes.ResourceNotFoundException{}
	}
	return &scheduler.DeleteScheduleOutput{}, nil
}

func TestSchedules_UpsertCreatesThenUpdates(t *testing.T) {
	api := &fakeScheduler{}
	s := NewSchedules(api, "group", "arn:sync", "arn:role")
	id := uuid.New()
	if err := s.Upsert(context.Background(), id, 30); err != nil {
		t.Fatal(err)
	}
	c := api.created[0]
	if *c.Name != "sync-"+id.String() || *c.ScheduleExpression != "rate(30 minutes)" || *c.GroupName != "group" || *c.Target.Arn != "arn:sync" {
		t.Errorf("created = %+v", c)
	}
	var job connector.Job
	if err := json.Unmarshal([]byte(*c.Target.Input), &job); err != nil || job.ConnectorID != id || job.Trigger != enum.RunTriggerSchedule {
		t.Errorf("target input = %s", *c.Target.Input)
	}
	if err := s.Upsert(context.Background(), id, 15); err != nil {
		t.Fatal(err)
	}
	if len(api.updates) != 1 || *api.updates[0].ScheduleExpression != "rate(15 minutes)" {
		t.Errorf("updates = %+v", api.updates)
	}
}

func TestSchedules_DeleteIgnoresAMissingSchedule(t *testing.T) {
	api := &fakeScheduler{missing: true}
	if err := NewSchedules(api, "g", "a", "r").Delete(context.Background(), uuid.New()); err != nil {
		t.Errorf("delete of a missing schedule = %v", err)
	}
	if len(api.deleted) != 1 || !strings.HasPrefix(api.deleted[0], "sync-") {
		t.Errorf("deleted = %v", api.deleted)
	}
}

type fakeLambda struct {
	in     *lambda.InvokeInput
	status int32
}

func (f *fakeLambda) Invoke(_ context.Context, in *lambda.InvokeInput, _ ...func(*lambda.Options)) (*lambda.InvokeOutput, error) {
	f.in = in
	return &lambda.InvokeOutput{StatusCode: f.status}, nil
}

func TestInvoker_StartsTheRunAsynchronously(t *testing.T) {
	api := &fakeLambda{status: 202}
	job := connector.Job{ConnectorID: uuid.New(), Trigger: enum.RunTriggerWebhook, ExternalIDs: []string{"1"}}
	if err := NewInvoker(api, "arn:sync").Invoke(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	var got connector.Job
	if string(api.in.InvocationType) != "Event" || json.Unmarshal(api.in.Payload, &got) != nil || got.ConnectorID != job.ConnectorID || got.ExternalIDs[0] != "1" {
		t.Errorf("invoke = %+v", api.in)
	}
	api.status = 500
	if err := NewInvoker(api, "arn:sync").Invoke(context.Background(), job); err == nil {
		t.Error("a status other than 202 must be an error")
	}
}
