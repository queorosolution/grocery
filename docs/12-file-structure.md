# 12 — File structure

One repository, two apps, shared contracts.

```text
grocery-platform/
├── README.md                     Index of everything
├── CLAUDE.md                     Rules for the coding agent
├── PROMPTS.md                    Build prompts in order
├── Makefile                      gen, check-contracts, test, build, deploy, migrate
├── .gitignore                    .env*, .next/, out/, .aws-sam/, node_modules/, *.local
├── .env.example                  Backend config (no secrets)
│
├── contracts/                    SINGLE SOURCE OF TRUTH
│   ├── enums.json                Roles, statuses, permissions, transitions, error codes
│   ├── schema.sql                Postgres schema (copied into backend/migrations/0001)
│   └── schema_test.sql           RLS and constraint tests
│
├── docs/                         00 … 16
│
├── backend/
│   ├── go.mod  go.sum
│   ├── template.yaml             AWS SAM: functions, queue, schedules, roles, alarms
│   ├── samconfig.toml            dev / prod parameters (no secrets)
│   ├── cmd/
│   │   ├── api/main.go           Lambda: HTTP API adapter → internal/http.Router
│   │   ├── worker/main.go        Lambda: SQS batch + direct invoke (sweep, rollup)
│   │   ├── timer/main.go         Lambda: order timers
│   │   ├── sync/main.go          Lambda: connector runs
│   │   ├── migrate/main.go       CLI: goose up/down with embedded migrations
│   │   ├── gen/main.go           CLI: generate code from contracts/enums.json
│   │   └── devserver/main.go     CLI: run the router on :8080 for local work
│   ├── migrations/
│   │   └── 0001_init.sql
│   ├── internal/
│   │   ├── shared/               Technical helpers (see 07 §2). No business rules.
│   │   │   ├── apperr/           error type + codes_gen.go
│   │   │   ├── authn/            access token, actor, cookies
│   │   │   ├── authz/            perm_gen.go + Require()
│   │   │   ├── clock/  config/  crypt/  db/  enum/  geo/
│   │   │   ├── httpx/            decode, JSON, errors, middleware, headers
│   │   │   ├── ids/  logx/  money/  ratelimit/  secrets/  validate/
│   │   ├── http/
│   │   │   ├── routes.go         EVERY route, its guard and handler — one table
│   │   │   └── router.go         builds the mux + middleware chain
│   │   ├── auth/  user/  team/  shop/
│   │   ├── catalog/  pricing/  billing/  delivery/  coupon/
│   │   ├── order/                service.go, flow_gen.go, hooks.go, view.go, repo.go, handler.go
│   │   ├── payment/              provider.go, payhere/, manual/
│   │   ├── refund/  finance/  stats/  audit/  upload/
│   │   ├── notify/               notify.go, templates.go, sms/, whatsapp/, walink/, logchannel/
│   │   ├── connector/            service.go, run.go, mapping.go, source/ (pgx + SSRF guard)
│   │   ├── cartclient/           client.go (+ fake for tests)
│   │   └── events/               outbox.go, publish.go, dispatch.go, sweep.go
│   └── test/
│       ├── testdb/               start schema in a fresh database per test package
│       ├── fakes/                fake cart service, fake PayHere notify builder, fake SMS
│       ├── integration/          flows: submit → review → pay → deliver
│       └── security/             see 11 §2
│
├── web/                          Next.js (App Router), output: "export"
│   ├── package.json  package-lock.json
│   ├── next.config.ts  tsconfig.json  eslint.config.js  vitest.config.ts
│   ├── .env.example
│   ├── public/
│   │   ├── _headers              Base security headers (CSP hashes added after build)
│   │   └── _redirects            GENERATED id-route rewrites (08 §11.2)
│   ├── scripts/                  shop-slugs.mjs, gen-redirects.mjs, csp-hashes.mjs
│   ├── src/                      See 08 §2 (app/, components/ui/, features/, lib/, i18n/)
│   └── e2e/                      Playwright smoke tests
│
└── .github/
    └── workflows/
        ├── ci.yml                Lint, test, security checks (11 §3)
        └── deploy.yml            SAM deploy + migrations (dev on main, prod on tag)
```

## Module file pattern (backend)

Every business module has the same files. Missing ones are fine; extra kinds are not.

| File | Holds | Max lines |
|---|---|---|
| `types.go` | Input / output structs, JSON tags | 200 |
| `handler.go` | HTTP handlers for this module | 250 |
| `service.go` | Business functions (split into `service_<area>.go` if long) | 300 |
| `repo.go` | SQL | 300 |
| `*_test.go` | Tests next to the code | — |

## Feature folder pattern (web)

```text
src/features/orders/
├── api.ts                useOrder, useOrders, useCancelOrder, usePay …
├── OrderTimeline.tsx
├── OrderLines.tsx
├── ActionBar.tsx         renders allowedActions
├── DeliveryCodeCard.tsx
└── PayButton.tsx
```

## Naming

| Thing | Style | Example |
|---|---|---|
| Go packages | short, lower case | `order`, `cartclient` |
| Go files | snake_case | `service_review.go` |
| React components | PascalCase file + export | `SlotPicker.tsx` |
| Hooks | `useX` | `useQuote` |
| API JSON | camelCase | `onlinePriceCents` |
| DB | snake_case | `online_price_cents` |
| Env vars | UPPER_SNAKE | `PLATFORM_DOMAIN` |
| SSM | `/<app>/<env>/<area>/<name>` | `/gp/prod/payhere/<id>` |
