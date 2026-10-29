// Command devserver runs the API on :8080 for local work. It refuses to start
// unless APP_ENV=local, so it can never serve real data by mistake.
package main

import (
	"bufio"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	apihttp "gp/internal/http"
	"gp/internal/shared/config"
	"gp/internal/shared/logx"
)

func main() {
	if err := run(); err != nil {
		slog.Error("devserver stopped", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	environ := os.Environ()
	if file := envFile(); file != "" {
		fromFile, err := readEnvFile(file)
		if err != nil {
			return err
		}
		// Real environment variables win over the file.
		environ = append(fromFile, environ...)
	}
	cfg, err := config.LoadFrom(environ)
	if err != nil {
		return err
	}
	if !cfg.IsLocal() {
		return errors.New("devserver only runs with APP_ENV=local")
	}
	logx.SetDefault(logx.New(cfg.LogLevel, os.Stdout))

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           apihttp.New(apihttp.Deps{Config: cfg}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	slog.Info("devserver listening", "addr", srv.Addr)
	return srv.ListenAndServe()
}

// envFile is ENV_FILE, or ".env" when that file exists.
func envFile() string {
	if f := os.Getenv("ENV_FILE"); f != "" {
		return f
	}
	if _, err := os.Stat(".env"); err == nil {
		return ".env"
	}
	return ""
}

// readEnvFile reads KEY=VALUE lines. Blank lines and # comments are skipped.
func readEnvFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		out = append(out, line)
	}
	return out, sc.Err()
}
