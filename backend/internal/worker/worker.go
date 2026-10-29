// Package worker is the logic of the worker Lambda: it runs queue messages
// through the event consumers and runs the scheduled jobs. cmd/worker only wires it.
package worker

import (
	"context"
	"encoding/json"
	"fmt"

	lambdaevents "github.com/aws/aws-lambda-go/events"

	"gp/internal/events"
	"gp/internal/shared/db"
	"gp/internal/shared/logx"
)

// Job is a scheduled task started by a direct invoke, e.g. {"type":"outbox.sweep"}.
type Job func(ctx context.Context) error

type Worker struct {
	db       *db.DB
	registry *events.Registry
	jobs     map[string]Job
}

func New(d *db.DB, r *events.Registry, jobs map[string]Job) *Worker {
	return &Worker{db: d, registry: r, jobs: jobs}
}

// response is both shapes of a worker answer: the SQS batch report, or nothing
// for a direct invoke (then Lambda only needs the error).
type response = lambdaevents.SQSEventResponse

// Handle takes the raw event because the queue and the scheduler send different
// shapes. A queue batch answers per message (partial failure): only the failed
// ones are returned to the queue. A direct invoke runs its job and returns the
// error, if any. The raw event is never logged.
func (w *Worker) Handle(ctx context.Context, raw json.RawMessage) (response, error) {
	var probe struct {
		Records []json.RawMessage `json:"Records"`
		Type    string            `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return response{}, fmt.Errorf("worker: unreadable event")
	}
	if len(probe.Records) > 0 {
		var batch lambdaevents.SQSEvent
		if err := json.Unmarshal(raw, &batch); err != nil {
			return response{}, fmt.Errorf("worker: unreadable queue event")
		}
		return w.handleBatch(ctx, batch), nil
	}
	return response{}, w.runJob(ctx, probe.Type)
}

func (w *Worker) handleBatch(ctx context.Context, batch lambdaevents.SQSEvent) response {
	var out response
	for _, rec := range batch.Records {
		if err := w.handleRecord(ctx, rec); err != nil {
			logx.From(ctx).Error("worker.message_failed", "messageId", rec.MessageId, "err", err)
			out.BatchItemFailures = append(out.BatchItemFailures,
				lambdaevents.SQSBatchItemFailure{ItemIdentifier: rec.MessageId})
		}
	}
	return out
}

// handleRecord parses one message and dispatches it. A message that cannot be
// read can never succeed; it is reported as failed so it reaches the DLQ, where
// the alarm makes someone look at it.
func (w *Worker) handleRecord(ctx context.Context, rec lambdaevents.SQSMessage) error {
	var m events.Message
	if err := json.Unmarshal([]byte(rec.Body), &m); err != nil {
		return fmt.Errorf("unreadable message body")
	}
	if m.ID == "" || !m.Type.Valid() {
		return fmt.Errorf("message without id or with an unknown type")
	}
	return w.registry.Dispatch(ctx, w.db, m)
}

func (w *Worker) runJob(ctx context.Context, name string) error {
	job, ok := w.jobs[name]
	if !ok {
		return fmt.Errorf("worker: unknown job %q", name)
	}
	if err := job(ctx); err != nil {
		return fmt.Errorf("worker: job %s: %w", name, err)
	}
	return nil
}
