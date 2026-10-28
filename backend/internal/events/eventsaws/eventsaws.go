// Package eventsaws connects the events package to AWS: SQS for queue messages
// and EventBridge Scheduler for order timers. It is the only place that knows
// the AWS shapes, so the events package can be tested without AWS.
package eventsaws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/scheduler"
	schedtypes "github.com/aws/aws-sdk-go-v2/service/scheduler/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"gp/internal/events"
	"gp/internal/shared/config"
)

// New builds the real queue and scheduler from the config. It returns nil, nil
// when no queue is configured (local runs): events then wait for the sweep.
func New(ctx context.Context, cfg *config.Config) (events.Queue, events.Scheduler, error) {
	if cfg.EventsQueueURL == "" {
		return nil, nil, nil
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("eventsaws: load AWS config: %w", err)
	}
	return &Queue{api: sqs.NewFromConfig(awsCfg), url: cfg.EventsQueueURL},
		&Scheduler{api: scheduler.NewFromConfig(awsCfg), group: cfg.SchedulerGroup,
			targetARN: cfg.TimerFunctionARN, roleARN: cfg.SchedulerRoleARN}, nil
}

// SQSAPI is the part of the SQS client we use, so tests can fake it.
type SQSAPI interface {
	SendMessageBatch(ctx context.Context, in *sqs.SendMessageBatchInput, opts ...func(*sqs.Options)) (*sqs.SendMessageBatchOutput, error)
}

type Queue struct {
	api SQSAPI
	url string
}

// NewQueue is for tests and for callers that build their own client.
func NewQueue(api SQSAPI, url string) *Queue { return &Queue{api: api, url: url} }

// Send puts up to 10 messages on the queue and returns the ids SQS refused.
func (q *Queue) Send(ctx context.Context, batch []events.Outgoing) ([]string, error) {
	entries := make([]sqstypes.SendMessageBatchRequestEntry, len(batch))
	for i, o := range batch {
		body := string(o.Body)
		entries[i] = sqstypes.SendMessageBatchRequestEntry{
			Id: &batch[i].ID, MessageBody: &body, DelaySeconds: int32(o.Delay / time.Second), // #nosec G115 -- at most 120
		}
	}
	out, err := q.api.SendMessageBatch(ctx, &sqs.SendMessageBatchInput{QueueUrl: &q.url, Entries: entries})
	if err != nil {
		return nil, fmt.Errorf("eventsaws: send batch: %w", err)
	}
	var failed []string
	for _, f := range out.Failed {
		if f.Id != nil {
			failed = append(failed, *f.Id)
		}
	}
	return failed, nil
}

// SchedulerAPI is the part of the Scheduler client we use.
type SchedulerAPI interface {
	CreateSchedule(ctx context.Context, in *scheduler.CreateScheduleInput, opts ...func(*scheduler.Options)) (*scheduler.CreateScheduleOutput, error)
}

type Scheduler struct {
	api                       SchedulerAPI
	group, targetARN, roleARN string
}

func NewScheduler(api SchedulerAPI, group, targetARN, roleARN string) *Scheduler {
	return &Scheduler{api: api, group: group, targetARN: targetARN, roleARN: roleARN}
}

// atLayout is the time format of an at() expression: UTC, no zone suffix.
const atLayout = "2006-01-02T15:04:05"

// timerInput is what the timer Lambda receives (docs/07 section 10).
type timerInput struct {
	OrderID    string `json:"orderId"`
	Kind       string `json:"kind"`
	RevisionNo int    `json:"revisionNo"`
}

// CreateSchedule makes a one-time schedule that deletes itself after it runs.
// ConflictException means the schedule exists already, which is success.
func (s *Scheduler) CreateSchedule(ctx context.Context, t events.Timer) error {
	input, err := json.Marshal(timerInput{OrderID: t.OrderID.String(), Kind: string(t.Kind), RevisionNo: t.RevisionNo})
	if err != nil {
		return err
	}
	name, expr, in := t.Name(), "at("+t.RunAt.UTC().Format(atLayout)+")", string(input)
	_, err = s.api.CreateSchedule(ctx, &scheduler.CreateScheduleInput{
		Name:                       &name,
		GroupName:                  &s.group,
		ScheduleExpression:         &expr,
		ScheduleExpressionTimezone: strPtr("UTC"),
		FlexibleTimeWindow:         &schedtypes.FlexibleTimeWindow{Mode: schedtypes.FlexibleTimeWindowModeOff},
		ActionAfterCompletion:      schedtypes.ActionAfterCompletionDelete,
		Target:                     &schedtypes.Target{Arn: &s.targetARN, RoleArn: &s.roleARN, Input: &in},
	})
	var conflict *schedtypes.ConflictException
	if errors.As(err, &conflict) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("eventsaws: create schedule %s: %w", name, err)
	}
	return nil
}

func strPtr(s string) *string { return &s }
