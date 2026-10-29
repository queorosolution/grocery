// Command worker handles queue messages and the direct schedule invokes
// (outbox sweep, stats roll-up). Stub: the real jobs arrive in later steps.
package main

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.Start(handle)
}

// handle takes the raw event because the queue and the scheduler send different
// shapes. It logs only a count, never the event. Nothing writes to the queue yet,
// so acknowledging every message here loses no work.
func handle(_ context.Context, raw json.RawMessage) (events.SQSEventResponse, error) {
	var batch events.SQSEvent
	_ = json.Unmarshal(raw, &batch)
	slog.Info("worker stub", "records", len(batch.Records))
	return events.SQSEventResponse{}, nil
}
