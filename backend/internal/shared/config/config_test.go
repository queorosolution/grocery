package config

import (
	"os"
	"strings"
	"testing"
)

// localEnv is .env.example, which must always load: the file is the contract for local runs.
func localEnv(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("../../../../.env.example")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, line := range strings.Split(string(raw), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
	}
	return out
}

func with(environ []string, overrides ...string) []string {
	skip := map[string]bool{}
	for _, o := range overrides {
		name, _, _ := strings.Cut(o, "=")
		skip[name] = true
	}
	var out []string
	for _, kv := range environ {
		if name, _, _ := strings.Cut(kv, "="); !skip[name] {
			out = append(out, kv)
		}
	}
	return append(out, overrides...)
}

func without(environ []string, names ...string) []string {
	var out []string
	for _, kv := range environ {
		keep := true
		for _, n := range names {
			if strings.HasPrefix(kv, n+"=") {
				keep = false
			}
		}
		if keep {
			out = append(out, kv)
		}
	}
	return out
}

// deployedEnv is a valid dev environment: .env.example without local-only
// values, plus what SAM provides.
func deployedEnv(t *testing.T) []string {
	t.Helper()
	env := localEnv(t)
	var out []string
	for _, kv := range env {
		if name, _, _ := strings.Cut(kv, "="); name == "DATABASE_URL" || name == "SECRETS_FROM_ENV" || strings.HasPrefix(name, "SECRET_") {
			continue
		}
		out = append(out, kv)
	}
	return with(out,
		"APP_ENV=dev", "WEB_ORIGIN=https://dev.nearshop.lk", "ALLOWED_ORIGINS=https://dev.nearshop.lk",
		"API_BASE_URL=https://api-dev.nearshop.lk", "SMS_API_URL=https://sms.example.lk/send", "SMS_SENDER_ID=NearShop",
		"EVENTS_QUEUE_URL=https://sqs.example/q", "SCHEDULER_GROUP=gp-dev", "SCHEDULER_ROLE_ARN=arn:role",
		"TIMER_FUNCTION_ARN=arn:timer", "SYNC_FUNCTION_ARN=arn:sync", "WORKER_FUNCTION_ARN=arn:worker",
		"R2_ACCOUNT_ID=acc", "R2_BUCKET=gp-images-dev", "SSM_PREFIX=/gp/dev")
}

func TestExampleEnvLoadsLocally(t *testing.T) {
	c, err := LoadFrom(localEnv(t))
	if err != nil {
		t.Fatal(err)
	}
	if !c.IsLocal() || c.PlatformName != "NearShop" || c.PlatformDomain != "nearshop.lk" || !c.SecretsFromEnv {
		t.Fatalf("unexpected %+v", c)
	}
	if len(c.AllowedOrigins) != 1 || c.SMSDailyCap != 50 || !c.FeatureSingleShop || c.FeatureWhatsApp {
		t.Fatalf("unexpected %+v", c)
	}
}

func TestDeployedEnvLoads(t *testing.T) {
	c, err := LoadFrom(deployedEnv(t))
	if err != nil {
		t.Fatal(err)
	}
	if c.IsLocal() || c.Env != EnvDev || c.SecretsFromEnv || c.DatabaseURL != "" {
		t.Fatalf("unexpected %+v", c)
	}
}

func TestFailsFastAndListsEveryProblem(t *testing.T) {
	_, err := LoadFrom(without(localEnv(t), "APP_NAME", "PLATFORM_NAME", "GOOGLE_CLIENT_ID"))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"APP_NAME is required", "PLATFORM_NAME is required", "GOOGLE_CLIENT_ID is required", "3 problem"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error misses %q:\n%v", want, err)
		}
	}
}

func TestRefusesLocalOnlySettingsOutsideLocal(t *testing.T) {
	for _, extra := range []string{"DATABASE_URL=postgres://x", "SECRETS_FROM_ENV=true", "SECRET_JWT_KEY=abc"} {
		_, err := LoadFrom(append(deployedEnv(t), extra))
		name, _, _ := strings.Cut(extra, "=")
		if err == nil || !strings.Contains(err.Error(), name+" is local-only") {
			t.Errorf("%s accepted in dev: %v", name, err)
		}
	}
}

func TestDeployedEnvNeedsWhatSAMProvides(t *testing.T) {
	_, err := LoadFrom(without(deployedEnv(t), "EVENTS_QUEUE_URL", "ADMIN_ALERT_PHONE", "SMS_API_URL"))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, name := range []string{"EVENTS_QUEUE_URL", "ADMIN_ALERT_PHONE", "SMS_API_URL"} {
		if !strings.Contains(err.Error(), name+" is required") {
			t.Errorf("missing %s not reported:\n%v", name, err)
		}
	}
}

func TestValueRules(t *testing.T) {
	base := deployedEnv(t)
	cases := map[string]string{
		"APP_ENV=staging":                          "APP_ENV must be one of",
		"PLATFORM_DOMAIN=https://nearshop.lk":      "PLATFORM_DOMAIN must be a bare domain",
		"WEB_ORIGIN=http://dev.nearshop.lk":        "WEB_ORIGIN must use https",
		"WEB_ORIGIN=https://dev.nearshop.lk/":      "must not end with a slash",
		"WEB_ORIGIN=https://dev.nearshop.lk/app":   "without a path",
		"ALLOWED_ORIGINS=*":                        "ALLOWED_ORIGINS",
		"ALLOWED_ORIGINS=https://*.nearshop.lk":    "wildcard",
		"ALLOWED_ORIGINS=https://a.lk, ftp://b.lk": "must use https",
		"SUPPORT_PHONE=0770000000":                 "SUPPORT_PHONE must look like",
		"SUPER_ADMIN_EMAILS=Admin <a@b.lk>":        "SUPER_ADMIN_EMAILS must be",
		"SSM_PREFIX=gp/dev":                        "SSM_PREFIX must look like",
		"SSM_PREFIX=/gp/dev/":                      "SSM_PREFIX must look like",
		"SMS_DAILY_CAP=0":                          "SMS_DAILY_CAP must be a positive",
		"FEATURE_WHATSAPP=yes":                     "FEATURE_WHATSAPP must be true or false",
		"LOG_LEVEL=loud":                           "LOG_LEVEL must be",
		"CART_API_BASE=https://user:pw@cart.lk/v1": "credentials",
	}
	for override, want := range cases {
		_, err := LoadFrom(with(base, override))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: want error containing %q, got %v", override, want, err)
		}
	}
}

func TestErrorsNeverContainValues(t *testing.T) {
	_, err := LoadFrom(with(deployedEnv(t), "SMS_API_URL=https://user:topsecret@sms.lk/send"))
	if err == nil || strings.Contains(err.Error(), "topsecret") {
		t.Fatalf("error must name the variable only: %v", err)
	}
}

func TestLocalAllowsHTTPLocalhost(t *testing.T) {
	if _, err := LoadFrom(with(localEnv(t), "WEB_ORIGIN=http://localhost:3000")); err != nil {
		t.Fatal(err)
	}
}
