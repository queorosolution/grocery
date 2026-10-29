// Command sync runs connector jobs: from the per-connector schedule, from
// "Sync now" and from the trigger URL. This file only wires things together.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/lambda"

	"gp/internal/shared/config"
	"gp/internal/shared/logx"
	"gp/internal/shared/secrets"
	"gp/internal/syncjob"
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
	job, closeAll, err := syncjob.Wire(ctx, cfg, sec)
	if err != nil {
		slog.Error("start-up failed", "error", err.Error())
		os.Exit(1)
	}
	defer closeAll()
	lambda.Start(job.Handle)
}
