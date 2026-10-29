// Package config reads every setting once at start-up and refuses to start on a
// missing or invalid value (docs/13-config.md). Error messages name the
// variable, never its value, because a value may be a secret.
package config

import (
	"log/slog"
	"os"

	"gp/internal/shared/logx"
)

type Env string

const (
	EnvLocal Env = "local"
	EnvDev   Env = "dev"
	EnvProd  Env = "prod"
)

type Config struct {
	AppName  string
	Env      Env
	LogLevel slog.Level

	PlatformName     string
	PlatformDomain   string
	WebOrigin        string
	AllowedOrigins   []string
	APIBaseURL       string
	ImgBaseURL       string
	SupportPhone     string
	SuperAdminEmails []string
	AdminAlertPhone  string
	GoogleClientID   string
	CartAPIBase      string
	SSMPrefix        string
	SMSAPIURL        string
	SMSSenderID      string
	SMSDailyCap      int

	FeatureSingleShop bool
	FeatureWhatsApp   bool
	FeatureManualPay  bool

	// Set by SAM; optional when running locally.
	EventsQueueURL    string
	SchedulerGroup    string
	SchedulerRoleARN  string
	TimerFunctionARN  string
	SyncFunctionARN   string
	WorkerFunctionARN string
	R2AccountID       string
	R2Bucket          string

	// Local only. Load refuses these in dev and prod.
	DatabaseURL    string
	SecretsFromEnv bool
}

// maxDomainLen keeps the order link inside one SMS (notify templates).
const maxDomainLen = 20

func (c *Config) IsLocal() bool { return c.Env == EnvLocal }

// Load reads the process environment.
func Load() (*Config, error) { return LoadFrom(os.Environ()) }

// LoadFrom reads "NAME=value" pairs, so tests never touch the real environment.
func LoadFrom(environ []string) (*Config, error) {
	r := newReader(environ)
	c := &Config{}

	c.AppName = r.str("APP_NAME", true)
	c.Env = r.oneOf("APP_ENV", EnvLocal, EnvDev, EnvProd)
	local := c.Env == EnvLocal
	deployed := !local // values SAM provides, or that only a real deployment needs

	level, err := logx.ParseLevel(r.get("LOG_LEVEL"))
	if err != nil {
		r.fail("LOG_LEVEL", "must be debug, info, warn or error")
	}
	c.LogLevel = level

	web := urlRule{required: true, allowHTTP: local}
	c.PlatformName = r.str("PLATFORM_NAME", true)
	c.PlatformDomain = r.domain("PLATFORM_DOMAIN")
	if len(c.PlatformDomain) > maxDomainLen {
		r.fail("PLATFORM_DOMAIN", "must be at most 20 characters, it is part of the links in SMS")
	}
	c.WebOrigin = r.url("WEB_ORIGIN", web)
	c.AllowedOrigins = r.urls("ALLOWED_ORIGINS", web)
	c.ImgBaseURL = r.url("IMG_BASE_URL", web)
	base := urlRule{required: true, allowHTTP: local, allowPath: true}
	c.APIBaseURL = r.url("API_BASE_URL", base)
	c.CartAPIBase = r.url("CART_API_BASE", base)
	c.SMSAPIURL = r.url("SMS_API_URL", urlRule{required: deployed, allowHTTP: local, allowPath: true})

	c.SupportPhone = r.phone("SUPPORT_PHONE", true)
	c.AdminAlertPhone = r.phone("ADMIN_ALERT_PHONE", deployed)
	c.SuperAdminEmails = r.emails("SUPER_ADMIN_EMAILS")
	c.GoogleClientID = r.str("GOOGLE_CLIENT_ID", true)
	c.SSMPrefix = r.ssmPrefix("SSM_PREFIX")
	c.SMSSenderID = r.str("SMS_SENDER_ID", deployed)
	c.SMSDailyCap = r.positiveInt("SMS_DAILY_CAP")

	c.FeatureSingleShop = r.boolean("FEATURE_SINGLE_SHOP")
	c.FeatureWhatsApp = r.boolean("FEATURE_WHATSAPP")
	c.FeatureManualPay = r.boolean("FEATURE_MANUAL_PAYMENT")

	c.EventsQueueURL = r.str("EVENTS_QUEUE_URL", deployed)
	c.SchedulerGroup = r.str("SCHEDULER_GROUP", deployed)
	c.SchedulerRoleARN = r.str("SCHEDULER_ROLE_ARN", deployed)
	c.TimerFunctionARN = r.str("TIMER_FUNCTION_ARN", deployed)
	c.SyncFunctionARN = r.str("SYNC_FUNCTION_ARN", deployed)
	c.WorkerFunctionARN = r.str("WORKER_FUNCTION_ARN", deployed)
	c.R2AccountID = r.str("R2_ACCOUNT_ID", deployed)
	c.R2Bucket = r.str("R2_BUCKET", deployed)

	if local {
		c.DatabaseURL = r.str("DATABASE_URL", false)
		if r.get("SECRETS_FROM_ENV") != "" {
			c.SecretsFromEnv = r.boolean("SECRETS_FROM_ENV")
		}
	} else {
		r.refuseLocalOnly([]string{"DATABASE_URL", "SECRETS_FROM_ENV"})
	}

	if err := r.err(); err != nil {
		return nil, err
	}
	return c, nil
}
