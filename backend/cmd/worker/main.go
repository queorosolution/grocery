// Command worker handles queue messages and the direct schedule invokes
// (outbox sweep, stats roll-up). This file only wires things together.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/lambda"

	"gp/internal/shared/config"
	"gp/internal/shared/logx"
	"gp/internal/shared/secrets"
	"gp/internal/worker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		// Config errors name variables, never values.
		slog.Error("invalid configuration", "error", err.Error())
		os.Exit(1)
	}
	logx.SetDefault(logx.New(cfg.LogLevel, os.Stdout))

	ctx := context.Background()
	ssm, err := secrets.NewSSM(ctx)
	if err != nil {
		slog.Error("secret store unavailable", "error", err.Error())
		os.Exit(1)
	}
	sec, err := secrets.New(cfg, ssm, os.Getenv)
	if err != nil {
		slog.Error("secret store unavailable", "error", err.Error())
		os.Exit(1)
	}
	w, closeAll, err := worker.Wire(ctx, cfg, sec)
	if err != nil {
		slog.Error("start-up failed", "error", err.Error())
		os.Exit(1)
	}
	defer closeAll()
	lambda.Start(w.Handle)
}
