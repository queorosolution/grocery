// Command api is the HTTP Lambda: API Gateway HTTP API v2 events go to the router.
// This file only wires things together.
package main

import (
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	apihttp "gp/internal/http"
	"gp/internal/shared/config"
	"gp/internal/shared/logx"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		// Config errors name variables, never values.
		slog.Error("invalid configuration", "error", err.Error())
		os.Exit(1)
	}
	logx.SetDefault(logx.New(cfg.LogLevel, os.Stdout))

	handler := apihttp.New(apihttp.Deps{Config: cfg})
	// The adapter never logs the raw Lambda event.
	lambda.Start(httpadapter.NewV2(handler).ProxyWithContext)
}
