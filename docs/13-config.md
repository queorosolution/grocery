# 13 — Configuration

- **Config values** (not secret) → environment variables, set by SAM per environment.
- **Secrets** → SSM `SecureString`. The environment only holds the SSM **name**.
- **Brand and domains are config.** Changing `PLATFORM_NAME` or `PLATFORM_DOMAIN` needs no code change.
- `config.Load()` reads everything once at start-up and **fails fast** on a missing or invalid value.

## 1. Backend variables

| Variable | Example | Notes |
|---|---|---|
| `APP_NAME` | `gp` | Prefix for AWS resources and SSM paths |
| `APP_ENV` | `dev` · `prod` · `local` | `local` allows secrets from env and log-only SMS |
| `PLATFORM_NAME` | `NearShop` | Used in UI (via bootstrap), SMS, receipts |
| `PLATFORM_DOMAIN` | `nearshop.lk` | Used to build links and allowed hosts. At most 20 characters (it must fit the 160-character SMS). |
| `WEB_ORIGIN` | `https://www.nearshop.lk` | Links in SMS, PayHere return / cancel URLs |
| `ALLOWED_ORIGINS` | `https://www.nearshop.lk,https://dev.nearshop.lk` | Exact CORS / CSRF list |
| `API_BASE_URL` | `https://api.nearshop.lk` | PayHere `notify_url` |
| `IMG_BASE_URL` | `https://img.nearshop.lk` | Allowed image prefix |
| `SUPPORT_PHONE` | `+94770000000` | Shown in the app |
| `SUPER_ADMIN_EMAILS` | `admin@example.com` | Comma list. Both this **and** a membership are needed. |
| `ADMIN_ALERT_PHONE` | `+94770000000` | Admin sign-in and alarm SMS |
| `GOOGLE_CLIENT_ID` | `…apps.googleusercontent.com` | Audience for ID tokens |
| `CART_API_BASE` | `https://api.cart.example.com/v1` | Your cart service |
| `SSM_PREFIX` | `/gp/dev` | All secret names start with this |
| `EVENTS_QUEUE_URL` | from SAM | SQS |
| `SCHEDULER_GROUP` | `gp-dev` | Schedule group |
| `SCHEDULER_ROLE_ARN` | from SAM | Role Scheduler uses to invoke targets |
| `TIMER_FUNCTION_ARN`, `SYNC_FUNCTION_ARN`, `WORKER_FUNCTION_ARN` | from SAM | Targets |
| `R2_ACCOUNT_ID`, `R2_BUCKET` | `…`, `gp-images-dev` | Uploads |
| `SMS_API_URL`, `SMS_SENDER_ID` | provider values | |
| `SMS_DAILY_CAP` | `500` | Global daily SMS cap |
| `FEATURE_SINGLE_SHOP` | `true` | Home opens the only active shop |
| `FEATURE_WHATSAPP` | `false` | Cloud API channel |
| `FEATURE_MANUAL_PAYMENT` | `true` | Allow `MANUAL` provider |
| `LOG_LEVEL` | `info` | |

**Local only** (`APP_ENV=local`): `DATABASE_URL`, `SECRETS_FROM_ENV=true` with `SECRET_<NAME>` variables. `config.Load()` refuses these in `dev` / `prod`.

## 2. Secrets in SSM (`<SSM_PREFIX>/…`)

| Name | Content |
|---|---|
| `db/url` | `postgresql://app_rw:…@ep-…-pooler…neon.tech/gp?sslmode=require` |
| `jwt/key` | 32+ random bytes, base64 |
| `enc/key` | 32 random bytes, base64 (`APP_ENC_KEY`) |
| `otp/pepper`, `ip/pepper` | 32 random bytes, base64 |
| `cart/platform-key` | `plk_…` |
| `turnstile/secret` | Cloudflare Turnstile secret |
| `sms/api-key` | SMS provider key |
| `whatsapp/token` | Cloud API token (phase 2) |
| `r2/keys` | `{"accessKeyId":"…","secretAccessKey":"…"}` |
| `payhere/<accountId>` | merchant secret (written by the admin API) |
| `connectors/<id>` | shop DB connection string (written by the admin API) |
| `connectors/<id>/trigger` | trigger HMAC secret (written by the admin API) |
| `pages/deploy-hook` | Cloudflare Pages deploy hook URL (site rebuild, D30) |

Create random values with `openssl rand -base64 32`.

## 3. Frontend variables (`web/.env*`, build time, all public)

| Variable | Example |
|---|---|
| `NEXT_PUBLIC_API_BASE` | `https://api.nearshop.lk/v1` |
| `NEXT_PUBLIC_CART_API_BASE` | `https://api.cart.example.com/v1` |
| `NEXT_PUBLIC_GOOGLE_CLIENT_ID` | `…apps.googleusercontent.com` |
| `NEXT_PUBLIC_MAPS_KEY` | browser key (referrer-restricted) |
| `NEXT_PUBLIC_TURNSTILE_SITE_KEY` | `0x4AAA…` |
| `NEXT_PUBLIC_IMG_BASE` | `https://img.nearshop.lk` |
| `NEXT_PUBLIC_PLATFORM_NAME` | `NearShop` (shown before bootstrap loads; also used in build-time page titles) |
| `NEXT_PUBLIC_PLATFORM_DOMAIN` | `nearshop.lk` (filled into `_headers` and Open Graph URLs at build) |
| `NEXT_PUBLIC_ACCENT` | `#0e7c66` |

Nothing secret may ever start with `NEXT_PUBLIC_` — everything in it ships to every browser. CI fails if any other `NEXT_PUBLIC_` name appears (XSS-09).
