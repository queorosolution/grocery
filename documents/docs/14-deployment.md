# 14 — Deployment

## 1. Accounts you need

| Service | For | Plan |
|---|---|---|
| AWS | Lambda, API Gateway, SQS, Scheduler, SSM, CloudWatch | Pay as you go + **Budget alert USD 5** |
| Cloudflare | DNS, Pages, R2, proxy, Turnstile | Free |
| Neon | Postgres | Free |
| Google Cloud | OAuth client (sign-in), Maps browser key | Free credit; set quotas |
| PayHere | Payments | Sandbox now, live later |
| SMS provider (Sri Lanka) | OTP + notifications | Prepaid |
| GitHub | Code, Actions | Free |

## 2. Environments

| | `dev` | `prod` |
|---|---|---|
| Deploy when | push to `main` | git tag `v*` + manual approval |
| Web | `dev.<domain>` | `www.<domain>` (apex redirects to `www`) |
| API | `api-dev.<domain>` | `api.<domain>` |
| DB | Neon branch `dev` | Neon `main` |
| PayHere | Sandbox | Sandbox → Live (config switch) |
| SMS | log only | real |

## 3. First-time setup (in this order)

1. **Domain** — buy it, add it to Cloudflare, set `PLATFORM_DOMAIN`.
2. **Neon** — project `gp`; create role `app_rw` in the console; create branch `dev`. Copy the **pooled** connection strings.
3. **Migrations** — `make migrate ENV=dev` (uses the owner URL from a GitHub secret / your shell, never from Lambda). Then run `contracts/schema_test.sql` on a scratch branch and expect 30 passes.
4. **SSM secrets** — create every name in `13-config.md` §2 for `dev` (`aws ssm put-parameter --type SecureString …`).
5. **Google** — OAuth client (web): authorized JavaScript origins = web origins. Maps browser key: referrer restriction + Maps JS + Places only + daily quota.
6. **Cloudflare R2** — bucket `gp-images-dev`, custom domain `img-dev.<domain>`, CORS for `PUT` from the web origin, API token → `r2/keys`.
7. **Cloudflare Turnstile** — widget for the web domains → site key (`NEXT_PUBLIC_TURNSTILE_SITE_KEY`) + secret (`turnstile/secret`).
8. **Cart service** — create one **platform** for this system with its Root API → store `plk_…` in `cart/platform-key`.
9. **Backend** — `sam build && sam deploy --config-env dev`. Map the API custom domain `api-dev.<domain>` (ACM certificate in the same region) and add a proxied CNAME in Cloudflare.
10. **Frontend** — Cloudflare Pages project from the repo: root `web`, framework preset "Next.js (Static HTML Export)", build `npm ci && npm run build`, output `out`, env vars from `13-config.md` §3. Custom domain `dev.<domain>`. Then **Settings → Builds → Deploy hooks** → create a hook → store its URL in SSM `pages/deploy-hook` (D30).
11. **PayHere sandbox** — add the dev web domain, get the merchant id + secret → admin console → Payment accounts (`SANDBOX`).
12. **First admin** — put your email in `SUPER_ADMIN_EMAILS`, sign in, the API creates your `SUPER_ADMIN` membership, set up TOTP.
13. **Krishna Super** — create shop → settings → hours → fee rule → slots → enable cart → connector (see `09-integrations.md` §5) → readiness check → accepting on.

## 4. SAM template contents (`backend/template.yaml`)

| Resource | Key settings |
|---|---|
| `Parameters` | `Env`, `PlatformName`, `PlatformDomain`, `AllowedOrigins`, `GoogleClientId`, `CartApiBase`, `SuperAdminEmails`, … (all values from `13` §1) |
| `Globals.Function` | `Runtime: provided.al2023`, `Architectures: [arm64]`, `Handler: bootstrap`, `MemorySize: 256`, `Tracing: PassThrough`, log retention via `AWS::Logs::LogGroup` 14 days |
| `ApiFunction` | `Events: HttpApi` catch-all `ANY /{proxy+}`; `ReservedConcurrentExecutions: 20`; policies from `10-security.md` §4.7 |
| `HttpApi` | `AWS::Serverless::HttpApi`, stage throttle 50 / 100, custom domain, **no** CORS config (the app sets CORS itself) |
| `WorkerFunction` | SQS event (batch 10, `FunctionResponseTypes: [ReportBatchItemFailures]`), reserved 5 |
| `TimerFunction` | invoked by Scheduler only, reserved 5 |
| `SyncFunction` | timeout 60 s, reserved 2 |
| `EventsQueue` + `EventsDLQ` | `maxReceiveCount: 3`, visibility timeout 6 × worker timeout |
| `SchedulerGroup` | `gp-<env>` |
| `SchedulerRole` | `lambda:InvokeFunction` on timer, sync, worker only |
| `OutboxSweepSchedule`, `StatsRollupSchedule` | cron from `04-architecture.md` §6 |
| `Alarms` + `AlertTopic` (SNS email) | from `10-security.md` §4.8 |

Build: `BuildMethod: makefile` per function (shared `go.mod`), `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -tags lambda.norpc -o $(ARTIFACTS_DIR)/bootstrap ./cmd/<name>`.

## 5. CI/CD

**`ci.yml`** (every pull request): all commands in `11-security-audit.md` §3, with a Postgres 16 service container.

**`deploy.yml`**:

```text
on push main  → build → migrate dev → sam deploy dev → smoke test (GET /v1/health, GET /v1/bootstrap)
on tag v*     → wait for approval (GitHub environment "prod") → migrate prod → sam deploy prod → smoke test
```

- AWS access through **GitHub OIDC** role (no long-lived AWS keys in GitHub).
- Owner DB URLs are GitHub environment secrets, used only by the migrate step.
- Actions pinned to commit SHAs; `permissions: contents: read, id-token: write`.

## 6. Local development

```bash
# 1. Postgres locally (Docker or a Neon dev branch)
docker run -d --name gp-pg -e POSTGRES_PASSWORD=pg -p 5432:5432 postgres:16
make migrate-local && make seed-local          # seed: 1 shop, products, slots, admin

# 2. API
cp .env.example .env && make dev-api            # cmd/devserver on :8080

# 3. Web
cd web && cp .env.example .env.local && npm ci && npm run dev   # next dev on :3000
```

- `localhost` counts as a secure context, so `Secure` cookies work on `http://localhost` in Chrome and Firefox.
- PayHere notify cannot reach localhost: use `make fake-payhere ORDER=<id> STATUS=2` (builds a signed notify with the sandbox secret and posts it).

## 7. Go-live checklist (prod)

- [ ] All of `11-security-audit.md` §4 ticked on `dev`.
- [ ] `prod` SSM secrets created (new random values, not copies of dev).
- [ ] Neon `main`: PITR on, restore drill done.
- [ ] PayHere **live** account approved for `www.<domain>`; live account added in admin; Krishna Super switched to it after one real small payment test (then refund it).
- [ ] SMS sender ID approved; daily cap set; admin alert phone set.
- [ ] Cart service plan `maxItems` > Krishna Super product count.
- [ ] Privacy policy, terms, refund policy pages live.
- [ ] Budget alert and every alarm tested.
- [ ] `FEATURE_SINGLE_SHOP=true` while only Krishna Super is active.

## 8. Rollback

- **Code:** redeploy the previous tag (`sam deploy` of the old build). Web: Cloudflare Pages "Rollback to this deployment" (note: a later deploy-hook build will deploy `main` again — pause shop edits while rolled back).
- **Database:** migrations are forward-only. A bad migration is fixed by a new migration. For data loss: Neon point-in-time restore to a new branch, then switch `db/url`.
- **Emergency stop:** admin → shop → "Accepting orders" off (all shops), or set API stage throttle to 0.

## 9. Go-live log

| Date | Version | Checked by | Notes |
|---|---|---|---|
| | | | |
