// Command api is the HTTP Lambda: API Gateway HTTP API v2 events go to the router.
// This file only wires things together.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	apihttp "gp/internal/http"
	"gp/internal/shared/config"
	"gp/internal/shared/logx"
	"gp/internal/shared/secrets"
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
	deps, closeDB, err := apihttp.Wire(ctx, cfg, sec)
	if err != nil {
		slog.Error("start-up failed", "error", err.Error())
		os.Exit(1)
	}
	defer closeDB()

	handler := apihttp.New(deps)
	// The adapter never logs the raw Lambda event.
	lambda.Start(httpadapter.NewV2(handler).ProxyWithContext)
}
