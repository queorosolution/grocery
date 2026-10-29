// Command timer is invoked by EventBridge Scheduler. Stub: the real job arrives in a later step.
package main

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/aws/aws-lambda-go/lambda"
)

func main() {
	lambda.Start(handle)
}

// handle ignores the event on purpose: it is never logged.
func handle(_ context.Context, _ json.RawMessage) error {
	slog.Info("timer stub")
	return nil
}
