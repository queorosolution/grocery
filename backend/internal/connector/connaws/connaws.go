// Package connaws connects the connector module to AWS: one recurring
// EventBridge Scheduler schedule per live connector, and the async invoke of
// the sync function. It is the only place that knows the AWS shapes, so the
// connector package can be tested without AWS.
package connaws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/scheduler"
	schedtypes "github.com/aws/aws-sdk-go-v2/service/scheduler/types"
	"github.com/google/uuid"

	"gp/internal/connector"
	"gp/internal/shared/config"
	"gp/internal/shared/enum"
)

// New builds the real schedules and invoker. It returns nils when the sync
// function is not configured (local runs): connectors then have no schedule and
// "Sync now" runs inside the request.
func New(ctx context.Context, cfg *config.Config) (connector.Schedules, connector.Invoker, error) {
	if cfg.SyncFunctionARN == "" {
		return nil, nil, nil
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("connaws: load AWS config: %w", err)
	}
	return NewSchedules(scheduler.NewFromConfig(awsCfg), cfg.SchedulerGroup, cfg.SyncFunctionARN, cfg.SchedulerRoleARN),
		NewInvoker(lambda.NewFromConfig(awsCfg), cfg.SyncFunctionARN), nil
}

// SchedulerAPI is the part of the Scheduler client we use.
type SchedulerAPI interface {
	CreateSchedule(ctx context.Context, in *scheduler.CreateScheduleInput, opts ...func(*scheduler.Options)) (*scheduler.CreateScheduleOutput, error)
	UpdateSchedule(ctx context.Context, in *scheduler.UpdateScheduleInput, opts ...func(*scheduler.Options)) (*scheduler.UpdateScheduleOutput, error)
	DeleteSchedule(ctx context.Context, in *scheduler.DeleteScheduleInput, opts ...func(*scheduler.Options)) (*scheduler.DeleteScheduleOutput, error)
}

type Schedules struct {
	api                       SchedulerAPI
	group, targetARN, roleARN string
}

func NewSchedules(api SchedulerAPI, group, targetARN, roleARN string) *Schedules {
	return &Schedules{api: api, group: group, targetARN: targetARN, roleARN: roleARN}
}

// Name is the schedule name of a connector (docs/04 section 7).
func Name(id uuid.UUID) string { return "sync-" + id.String() }

// Upsert creates the schedule, or changes its interval when it exists.
// The handler skips runs outside open hours, so the schedule itself is plain.
func (s *Schedules) Upsert(ctx context.Context, id uuid.UUID, intervalMin int) error {
	input, err := json.Marshal(connector.Job{ConnectorID: id, Trigger: enum.RunTriggerSchedule})
	if err != nil {
		return err
	}
	name, expr, in, tz := Name(id), fmt.Sprintf("rate(%d minutes)", intervalMin), string(input), "UTC"
	window := &schedtypes.FlexibleTimeWindow{Mode: schedtypes.FlexibleTimeWindowModeOff}
	target := &schedtypes.Target{Arn: &s.targetARN, RoleArn: &s.roleARN, Input: &in}
	_, err = s.api.CreateSchedule(ctx, &scheduler.CreateScheduleInput{
		Name: &name, GroupName: &s.group, ScheduleExpression: &expr, ScheduleExpressionTimezone: &tz,
		FlexibleTimeWindow: window, Target: target,
	})
	var conflict *schedtypes.ConflictException
	if errors.As(err, &conflict) {
		_, err = s.api.UpdateSchedule(ctx, &scheduler.UpdateScheduleInput{
			Name: &name, GroupName: &s.group, ScheduleExpression: &expr, ScheduleExpressionTimezone: &tz,
			FlexibleTimeWindow: window, Target: target,
		})
	}
	if err != nil {
		return fmt.Errorf("connaws: schedule %s: %w", name, err)
	}
	return nil
}

// Delete removes the schedule. One that is already gone is fine.
func (s *Schedules) Delete(ctx context.Context, id uuid.UUID) error {
	name := Name(id)
	_, err := s.api.DeleteSchedule(ctx, &scheduler.DeleteScheduleInput{Name: &name, GroupName: &s.group})
	var missing *schedtypes.ResourceNotFoundException
	if err != nil && !errors.As(err, &missing) {
		return fmt.Errorf("connaws: delete schedule %s: %w", name, err)
	}
	return nil
}

// LambdaAPI is the part of the Lambda client we use.
type LambdaAPI interface {
	Invoke(ctx context.Context, in *lambda.InvokeInput, opts ...func(*lambda.Options)) (*lambda.InvokeOutput, error)
}

type Invoker struct {
	api LambdaAPI
	arn string
}

func NewInvoker(api LambdaAPI, functionARN string) *Invoker {
	return &Invoker{api: api, arn: functionARN}
}

// Invoke starts the sync function and returns at once (an "Event" invoke).
func (i *Invoker) Invoke(ctx context.Context, job connector.Job) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	out, err := i.api.Invoke(ctx, &lambda.InvokeInput{FunctionName: &i.arn, InvocationType: lambdatypes.InvocationTypeEvent, Payload: payload})
	if err != nil {
		return fmt.Errorf("connaws: invoke sync: %w", err)
	}
	if out.StatusCode != 202 {
		return fmt.Errorf("connaws: invoke sync: status %d", out.StatusCode)
	}
	return nil
}
