# PROMPTS.md — build steps for Claude Code

**How to use**
- Run prompts **in order**, one per session (or one at a time with `/clear` between them).
- Paste the block under "Prompt" as-is. Each prompt is small on purpose: it keeps the agent focused and the system flow unbroken.
- After each prompt: run `make test lint`, read the summary, commit.
- If the agent asks a question, answer it and update the doc it points to.

Every prompt implicitly includes: *"Follow CLAUDE.md. Read only the listed files. If something is unclear, stop and ask."*

Web prompts (P06, P28–P34) build a **Next.js static export** with `"use client"` screens — see `docs/08-frontend-design.md` §11 before any web work.

---

## Phase 0 — Foundations

### P01 — Repository skeleton and contract generator
**Read:** `CLAUDE.md`, `docs/12-file-structure.md`, `docs/07-backend-design.md` §5, `contracts/enums.json`, `contracts/schema.sql`

**Prompt**
> Create the repository skeleton from `docs/12-file-structure.md` (folders, `go.mod` with module `gp`, Go 1.24, `Makefile`, `.gitignore`). Write `backend/cmd/gen` (standard library only) that reads `contracts/enums.json` and writes: `internal/shared/enum/enum_gen.go` (typed string constants, `Values()`, `Valid()` per enum), `internal/shared/authz/perm_gen.go` (role → permissions map + conditional permissions), `internal/order/flow_gen.go` (transition table + `Allowed(from, to, role)` + final statuses), `internal/shared/apperr/codes_gen.go` (code → HTTP status + message), and `web/src/lib/enums.gen.ts`, `perms.gen.ts`, `errors.gen.ts`. Add `make gen`. Add `make check-contracts` that parses every `CHECK (col IN (...))` in `contracts/schema.sql` and fails if a list differs from the matching enum (write the column → enum map in the checker). Add tests for the generator.

**Done when:** `make gen` is repeatable (no diff on second run); `make check-contracts` passes; tests pass.

### P02 — Shared helpers
**Read:** `docs/07-backend-design.md` §2 and §14, `docs/13-config.md`, `docs/10-security.md` §4.4

**Prompt**
> Implement `internal/shared/{config, secrets, logx, apperr, clock, ids, money, geo, validate, crypt}` exactly as listed in `07` §2. `config.Load()` fails fast on missing values and refuses local-only variables outside `APP_ENV=local`. `secrets.Get` reads SSM `SecureString` with a 5-minute cache (env fallback only in local). `logx` redacts keys and phone-like values. `money`: `Cents` type, `RoundRupee` (half up), `ApplyBP`, `MulQty`, `FormatLKR`. `crypt`: AES-256-GCM `Seal/Open` with a key-version byte, `HMAC`, `Equal` (constant time), `RandomDigits(n)`. `ids`: ULID event ids `evt_…`, order codes (Crockford base32), 32-byte tokens. Unit tests for all.

**Done when:** tests pass, including `TestRedaction` (DATA-01).

### P03 — Database access and migrations
**Read:** `docs/05-data-model.md` §5, `docs/07-backend-design.md` §3 (Scope), `contracts/schema.sql`, `contracts/schema_test.sql`

**Prompt**
> Copy `contracts/schema.sql` to `backend/migrations/0001_init.sql` (goose format). Write `cmd/migrate` (goose, embedded files). Write `internal/shared/db`: pgx pool from the `db/url` secret (max 2 conns per Lambda), `Scope` type, `db.Tx(ctx, scope, fn)` that begins, runs the three `set_config(..., true)` calls, runs `fn`, commits, then runs `AfterCommit` hooks. At start-up, check `SELECT current_user` is `app_rw` (except local) — refuse to start otherwise (INFRA-05). Write `test/testdb` that creates a fresh database per package, applies the schema, and connects as `app_rw`. Add a CI step that runs `schema_test.sql`.

**Done when:** a test proves RLS works through `db.Tx` (customer scope cannot read another customer's order); `schema_test.sql` passes 30/30.

### P04 — HTTP layer and router
**Read:** `docs/06-api.md` §1, `docs/07-backend-design.md` §3, `docs/10-security.md` §5 (API headers), `docs/11-security-audit.md` CSRF-01, CORS-01, CORS-02

**Prompt**
> Implement `internal/shared/httpx` (Decode with 64 KB limit and `DisallowUnknownFields`, JSON, Error mapping `apperr` → error shape, security headers) and the middleware chain in `07` §3. Build `internal/http/routes.go` as a table: method, pattern, guard (`public`, `user`, `perm:<name>`, `admin`, `sig`), handler. Add a test that fails if any route has no guard. Add `GET /v1/health` and `GET /v1/bootstrap` (platform + features, `me: null` for now). Write `cmd/api` (Lambda HTTP API v2 adapter) and `cmd/devserver` (:8080).

**Done when:** CSRF and CORS tests (CSRF-01, CORS-01, CORS-02) pass; `curl localhost:8080/v1/health` works.

### P05 — SAM template and CI
**Read:** `docs/04-architecture.md` §3, §6, `docs/14-deployment.md` §4–5, `docs/10-security.md` §4.7

**Prompt**
> Write `backend/template.yaml` and `samconfig.toml` with every resource in `14` §4 (functions may have stub handlers for worker/timer/sync for now). IAM exactly as `10` §4.7. Write `.github/workflows/ci.yml` (all checks in `11` §3, Postgres 16 service) and `deploy.yml` (OIDC, migrate, deploy, smoke test). Pin actions to SHAs.

**Done when:** `sam validate --lint` passes; CI runs green on a PR.

### P06 — Next.js skeleton, static export plumbing, design system
**Read:** `docs/08-frontend-design.md` §1–3, §5 rules 1–2, §9, §11, `docs/10-security.md` §5 (web headers), `docs/13-config.md` §3

**Prompt**
> Create the Next.js (App Router, TypeScript strict, `src/` dir) app in `web/` with only the allowed packages. `next.config.ts`: `output: "export"` only for production builds, `trailingSlash: true`, `images.unoptimized: true`. Root `layout.tsx` + `providers.tsx` (`"use client"`). Add `components/ui/tokens.css` and the `components/ui/` components from `08` §3.2 (all except `MapPicker`, `SearchBar2Way`, charts — later). Add `lib/api.ts` (credentials, `X-Requested-With`, error mapping, one refresh retry), `lib/keys.ts`, `lib/env.ts` (typed `NEXT_PUBLIC_*`), `lib/useRouteId.ts`, `lib/i18n.ts`, `lib/money.ts`, `lib/time.ts`, `lib/safeUrl.ts` (+ tests for XSS-03, XSS-04, XSS-05). Add the three scripts: `shop-slugs.mjs` (falls back to an empty list if the API is down, so builds never fail), `gen-redirects.mjs` (writes `public/_redirects` from id-route folders, `08` §11.2), `csp-hashes.mjs` (post-build per-page CSP hashes into `out/_headers`, `--check` mode fails on unhashed inline scripts). `npm run build` = `next build` + both scripts. Placeholder pages: `/`, `/s/[slug]`, `/order/[id]`, `/console/shop/[shopId]/order/[orderId]`, and `app/not-found.tsx` that renders the shop placeholder for `/s/<slug>`. ESLint with `react/no-danger`, `no-eval`, and a rule banning `params` use in id pages. CI check for unknown `NEXT_PUBLIC_` names (XSS-09).

**Done when:** `npm run build` passes and `out/` exists; first-load JS for `/` < 180 KB gzip; deploy a **Cloudflare Pages preview** and confirm by hand: `/order/abc123/` shows the order placeholder with id `abc123`; `/s/new-shop/` (not pre-built) renders through `not-found.tsx`; no CSP errors in the browser console (XSS-08). Report the results before moving on.

---

## Phase 1 — Backend

### P07 — Google sign-in and sessions
**Read:** `docs/06-api.md` §4, `docs/09-integrations.md` §4.1, `docs/10-security.md` §4.1, `docs/11-security-audit.md` AUTHN-01..04, AUTHN-08

**Prompt**
> Implement the `auth` module: `POST /auth/google`, `/auth/refresh`, `/auth/logout`, the `authn` middleware (access JWT HS256 pinned), cookies, refresh rotation with family reuse detection, and the `me` object in `/bootstrap` and `GET /me`. Auth writes run with system scope. Rate limit sign-in.

**Done when:** tests for AUTHN-01..04 and AUTHN-08 pass.

### P08 — Admin TOTP, invites and team
**Read:** `docs/06-api.md` §4 (TOTP), §8 (team), §10 (people), `docs/01-srs-actors.md` §2, `docs/11-security-audit.md` AUTHN-05..07, AUTHZ-04, AUTHZ-07

**Prompt**
> Implement TOTP setup / verify, the `admin` guard, `authz.Require` with shop membership loading for `{shopId}` routes (out of scope → 404), invites (create, accept on Google sign-in), and the `team` module for owners. Write the generated authz matrix test skeleton in `test/security/authz_matrix_test.go`.

**Done when:** AUTHN-05..07, AUTHZ-04, AUTHZ-07 tests pass.

### P09 — Shops, settings, payment accounts, audit
**Read:** `docs/06-api.md` §10 (shops, payment accounts), `docs/02-srs-features.md` F3, `docs/05-data-model.md` §3, `docs/10-security.md` §6

**Prompt**
> Implement `shop` (create, profile, settings, hours, accepting switch, readiness, `CanTakeOrders(now)` using Colombo time), public `GET /shops?fields=slug`, and `audit.Write/List`. Shop create and changes to slug / name / logo / status add a `site.rebuild` event (the consumer comes in P12). Payment accounts: secrets are write-only and stored in SSM under `payhere/<accountId>`. Every admin change writes an audit row.

**Done when:** tests for readiness rules, open-hours edge cases (midnight, closed days), and that no API response contains a secret (DATA-08 style).

### P10 — Delivery fees and slots
**Read:** `docs/07-backend-design.md` §7, `docs/03-order-flow.md` §10, `docs/06-api.md` §3 (slots), §10 (fee rules, slots)

**Prompt**
> Implement `delivery`: pure `Fee()` with the example from `07` §7 as a test, fee rule admin endpoints + preview, slot admin endpoints + copy-week, public `GET /shops/{slug}/slots`, and `Book` / `Release` with the atomic queries.

**Done when:** `TestFee_*` and `TestSlot_Race` pass.

### P11 — Customer profile, phone OTP, addresses
**Read:** `docs/06-api.md` §5, `docs/02-srs-features.md` F1, F2, `docs/10-security.md` §4.6, `docs/11-security-audit.md` AUTHN-09, BIZ-13

**Prompt**
> Implement the `user` module and a minimal `notify` with the log channel (real SMS comes in P25). Phone send needs Turnstile verification (server-side call). OTP: HMAC with pepper, 10 min, 5 tries, caps per user and phone, `+94` only. Addresses return `deliveringShops` using shop location + fee rule `max_km`.

**Done when:** AUTHN-09 and BIZ-13 tests pass.

### P12 — Events, outbox and worker skeleton
**Read:** `docs/07-backend-design.md` §10, `docs/04-architecture.md` §5–6

**Prompt**
> Implement `events` (`Add` in-transaction, publish after commit via SQS batch, `timer.schedule` → Scheduler `CreateSchedule` with conflict = success, `Sweep`, `Dispatch` registry with `processed_events` idempotency) and `cmd/worker` (SQS batch with partial failures + direct-invoke `outbox.sweep` and `stats.rollup`). Consumers can be empty stubs, except `site.OnRebuild` (120 s delay, skip if a newer `site.rebuild` exists, POST the deploy hook from SSM).

**Done when:** tests prove an event written in a rolled-back transaction is never published, and a duplicate SQS delivery runs a consumer once.

### P13 — Catalog admin and pricing engine
**Read:** `docs/07-backend-design.md` §6, `docs/02-srs-features.md` F4, `docs/06-api.md` §10 (catalog), §11

**Prompt**
> Implement `pricing` (pure `OnlinePrice` + `SetShopPrice` + `Reprice(shopID)` writing `price_changes` and `catalog.changed`), admin catalog endpoints (categories, products, shop products, category markups), console `PATCH /products/{id}` (manual products only), and `upload.Presign` (jpeg/png/webp, ≤ 2 MB).

**Done when:** `TestOnlinePrice_*`, AUTHZ-08, XSS-04, XSS-07 tests pass.

### P14 — Public catalog: home, search, shop, buy-again
**Read:** `docs/06-api.md` §3, §5 (buy-again), `docs/05-data-model.md` §6, `docs/04-architecture.md` §4

**Prompt**
> Implement public `GET /home`, `/search`, `/shops/{slug}`, `/shops/{slug}/products`, `/categories` with cache headers, and `GET /shops/{slug}/buy-again`. Search uses `SET LOCAL pg_trgm.word_similarity_threshold = 0.4` and `$q <% search_doc`. Round home `lat/lng` to 2 decimals server-side.

**Done when:** typo search test passes; SQLI-03 test passes; each endpoint is one DB round-trip where possible.

### P15 — Cart service client and price sync
**Read:** `docs/09-integrations.md` §1, `docs/06-api.md` §10 (cart-tenant)

**Prompt**
> Implement `cartclient` (platform key + `X-Tenant-Id`, timeouts 3 s, typed errors → `CART_UNAVAILABLE`) with a fake in `test/fakes`. Admin `POST /shops/{id}/cart-tenant`. Worker consumer for `catalog.changed` → `POST /v1/items/batch` in chunks of 25.

**Done when:** tests with the fake cart pass, including chunking and retry on 5xx.

### P16 — Connector part 1: connect, test, map, preview
**Read:** `docs/07-backend-design.md` §11, `docs/09-integrations.md` §5, `docs/02-srs-features.md` F5, `docs/11-security-audit.md` SSRF-01, SSRF-02, SQLI-02, DATA-08

**Prompt**
> Implement connector save (connection string → SSM, masked display), the SSRF guard (resolve, block ranges, connect to resolved IP with TLS server name, port 5432 only, `sslmode=require`, read-only session, 20 s timeout), `test`, `tables`, `columns`, `mapping` (validated against `information_schema` + regex) and `preview` (20 rows + planned action per row).

**Done when:** SSRF-01, SSRF-02, SQLI-02, DATA-08 tests pass.

### P17 — Connector part 2: runs, schedule, trigger, issues
**Read:** `docs/07-backend-design.md` §11 (flowchart), `docs/06-api.md` §7 (trigger), §10 (sync issues), `docs/11-security-audit.md` BIZ-15, BIZ-16

**Prompt**
> Implement `connector.Run` exactly as the flowchart (no overlap, open-hours rule, incremental or hash mode, matching, auto-create, holds, missing rows → unavailable, batches of 200, counts, 3 failures → `connector.failed`), `cmd/sync`, the per-connector Scheduler schedule (create / update / delete on activate / pause / interval change), `POST /connectors/{id}/trigger` (HMAC, ±5 min, rate limit, async invoke), trigger-secret rotation, and sync-issue resolution.

**Done when:** `TestConnector_*` and BIZ-15, BIZ-16 tests pass.

### P18 — Bill builder and quote
**Read:** `docs/07-backend-design.md` §8, `docs/06-api.md` §6 (quote), `docs/03-order-flow.md` §6

**Prompt**
> Implement pure `billing.Build` with table tests (`TestBuild_*`) and `POST /bills/quote` (loads cart via `cartclient`, shop, products, fee rule, slots; no writes). Integer quantities only (D28).

**Done when:** `TestBuild_*`, BIZ-01, BIZ-02 tests pass.

### P19 — Submit bill, order view, idempotency
**Read:** `docs/03-order-flow.md` §3 (T01), §4, §7, `docs/06-api.md` §6, `docs/07-backend-design.md` §9

**Prompt**
> Implement the idempotency middleware (`idempotency_keys`), `order.Transition` core (lock, generated table check, version, hooks map, events, audit), `POST /bills` (T01 hook: book slot, insert order + lines, events, review timer, delete cart after commit), order view mappers per viewer with server-computed `allowedActions`, and `GET /orders`, `GET /orders/{id}`.

**Done when:** `TestSubmit_*`, `TestTransition_Table`, `TestTransition_StaleVersion`, AUTHZ-01, AUTHZ-06, BIZ-05, BIZ-12 tests pass.

### P20 — Review, revision, cancel and timers
**Read:** `docs/03-order-flow.md` §3 (T02–T09, T12), §5, §8, `docs/06-api.md` §6, §8 (review body)

**Prompt**
> Implement console order list (`ETag`) and detail, `POST …/review` (approve / revise / reject), customer accept-revision and cancel, freezing of totals, and `cmd/timer` with no-op on moved orders.

**Done when:** `TestReview_*`, `TestTimer_NoOpWhenMoved`, BIZ-10, BIZ-17, AUTHZ-02 tests pass.

### P21 — Coupons
**Read:** `docs/02-srs-features.md` F10, `docs/03-order-flow.md` §11, `docs/06-api.md` §6 (coupon), §10 (coupons)

**Prompt**
> Implement coupon admin endpoints and apply / remove on `AWAITING_PAYMENT` with atomic reserve, per-user limit, try limit, caps via `billing.Build`, and release on expire / cancel hooks.

**Done when:** `TestCoupon_*`, BIZ-03, BIZ-14 tests pass.

### P22 — Payments and ledger
**Read:** `docs/09-integrations.md` §2, `docs/07-backend-design.md` §12, `docs/05-data-model.md` §4, `docs/11-security-audit.md` BIZ-06..09, DATA-03

**Prompt**
> Implement the `payment` module (provider interface, PayHere checkout fields + hash, notify handler with every check in order, manual provider + admin mark-paid) and `finance.PostPaid`. Add `make fake-payhere`.

**Done when:** `TestPayHere_Notify_*`, `TestLedger_*` (paid part), BIZ-06..09, DATA-03 tests pass.

### P23 — Fulfilment and delivery code
**Read:** `docs/03-order-flow.md` §3 (T13–T18), §9, `docs/06-api.md` §8 (status, driver), §9

**Prompt**
> Implement status moves for picking / packing / out for delivery / retry, driver assignment with `driver_source` rules, delivery code create (encrypted) and verify (5 tries, lock, admin unlock), driver endpoints, `finance.PostDelivered`, and the buy-again consumer on `order.delivered`.

**Done when:** `TestDeliveryCode_*`, AUTHZ-03, BIZ-11, DATA-04, full ledger sum test pass.

### P24 — Refunds, tickets, payouts, admin order tools
**Read:** `docs/02-srs-features.md` F13, F16, `docs/03-order-flow.md` (T19 and related actions), `docs/06-api.md` §8 (refund requests), §10 (orders, refunds, tickets, payouts)

**Prompt**
> Implement refund requests (shop), admin decide / paid-out with ledger reversals, T19, support tickets, payouts (amount from ledger), and admin order search / transition / unlock.

**Done when:** tests for refund approval paths, payout amount and ledger reversal pass.

### P25 — Notifications
**Read:** `docs/09-integrations.md` §3, `docs/07-backend-design.md` §13, `docs/02-srs-features.md` F14

**Prompt**
> Implement templates (≤ 160 chars test), the SMS HTTP adapter (one provider file), WA link channel, daily cap + 80% alert, and the worker consumers for every event in `04` §5.

**Done when:** `TestTemplates_Max160`, cap tests, HDR-01 test pass.

### P26 — Stats and dashboards
**Read:** `docs/02-srs-features.md` F15, `docs/05-data-model.md` §7, `docs/06-api.md` §8 (dashboard), §9 (stats), §10 (insight)

**Prompt**
> Implement `stats.Apply` for order events, nightly rollup + retention cleanup, and the shop, driver and admin dashboard endpoints reading `shop_daily_stats`.

**Done when:** dashboard numbers match a seeded scenario test.

### P27 — Security test suite completion
**Read:** `docs/11-security-audit.md` (all), `docs/15-testing.md` §2

**Prompt**
> Complete `test/security/*`: generate the authz matrix from `routes.go` (every route × role × own / other scope), webhook, SSRF, race and auth suites. Run every C and H check. Report any gap instead of skipping it.

**Done when:** all C / H rows in `11` §1 have a passing automated test.

---

## Phase 1 — Web

### P28 — Sign-in, guards, onboarding
**Read:** `docs/08-frontend-design.md` §4 (customer, sign-in), §5, §11, `docs/06-api.md` §4–5

**Prompt**
> Build the sign-in modal (Google Identity Services, loaded on demand), `useMe` from `/bootstrap`, client-side guards in `app/console/layout.tsx` (`RequireUser`, `RequireRole`, `RequireTotp` with TOTP setup / code screens), onboarding (name → phone with Turnstile + code → address with `MapPicker`), and the account pages. All screens `"use client"`.

**Done when:** component tests pass; `safeNext` used for returns.

### P29 — Home, search, shop page, cart
**Read:** `docs/08-frontend-design.md` §3.2, §4, §5, §6, §11.1

**Prompt**
> Build `SearchBar2Way`, home sections, search results, the shop page (`/s/[slug]`: build-time `generateMetadata` with shop name, description and logo for link previews; client body with header in shop theme color, category chips, product grid, buy-again row, sticky cart bar) and reuse the same shop screen inside `not-found.tsx`. Add `CartProvider` + `cartApi` + `CartSheet` + `SwitchShopDialog` with debounced quantity updates.

**Done when:** home = 2 API calls, shop page = 2 (+1 buy-again when signed in); tests for the switch-shop rule; a shop link pasted in WhatsApp shows its name and logo (on the preview deploy).

### P30 — Bill page and order page
**Read:** `docs/08-frontend-design.md` §5, §7, §8, §11.2, `docs/06-api.md` §6

**Prompt**
> Build `/checkout` (quote on open / address / slot change only, substitution choice per line, totals, slot picker, phone check, submit with `Idempotency-Key`) and `/order/[id]` (id from `useRouteId()`; timeline, revision compare, coupon box, `ActionBar` from `allowedActions`, `PayButton` with allowed-action check, delivery code card, report problem, `usePolling`).

**Done when:** tests for `ActionBar` and `PayButton` (XSS-05) pass; `npm run gen:redirects` re-run; `/order/<real id>/` opens the right order on the preview deploy.

### P31 — Shop console
**Read:** `docs/08-frontend-design.md` §4 (console), §11.2, `docs/06-api.md` §8, `docs/01-srs-actors.md` UC-SHP-*

**Prompt**
> Build the shop console (`/console/shop/[shopId]/orders` and `/console/shop/[shopId]/order/[orderId]`, ids from `useRouteId()`): order queue with tabs and new-bill sound, review panel (approve / revise / reject), pick list, driver assignment, products table + edit dialog, team, dashboard (stat tiles + `MiniBarChart`), payouts.

**Done when:** review flow component tests pass; polling stops when hidden; `npm run gen:redirects` re-run and console id pages work on the preview deploy.

### P32 — Driver screen
**Read:** `docs/08-frontend-design.md` §4 (driver), `docs/06-api.md` §9

**Prompt**
> Build `/console/driver`: today's deliveries by slot, call and map buttons, big code entry with attempts left, "Couldn't deliver" with reason.

**Done when:** works at 360 px width; tests for code entry states.

### P33 — Admin console
**Read:** `docs/08-frontend-design.md` §4 (admin), §11, `docs/06-api.md` §10, `docs/09-integrations.md` §5, §8

**Prompt**
> Build the admin console: shop setup at `/console/admin/shop/[id]?tab=…` (profile, settings, hours, slots incl. copy week, fees with preview, payment accounts, cart tenant, connector wizard: connect → test → table → map → preview → activate → trigger secret shown once), readiness checklist, catalog, sync issues, coupons, orders, refunds, tickets, payouts, users / invites, audit log, dashboard.

**Done when:** connector wizard tests pass; secrets never rendered after save; creating a test shop starts one Pages rebuild (deploy hook) and its `/s/<slug>` page works before and after the rebuild.

### P34 — E2E, accessibility, performance
**Read:** `docs/15-testing.md` §3, `docs/02-srs-features.md` NFR-PERF-*, NFR-A11Y-01

**Prompt**
> Write the 5 Playwright smoke flows and `e2e/xss.spec.ts`. Run an accessibility pass (labels, focus, contrast) and check the bundle budget. Fix only what these checks find.

**Done when:** all E2E pass on `dev` (including the rewrite and new-shop checks in `15-testing.md` §3); no CSP errors on any page (XSS-08); bundle < 180 KB gzip.

### P35 — Pre-go-live audit
**Read:** `docs/11-security-audit.md` §4, `docs/14-deployment.md` §7

**Prompt**
> Walk through `11` §4 and `14` §7 with me. For each item, tell me exactly what to run or click, and record results in the go-live log. Do not change application code in this step; list any problem as a new task.

**Done when:** every box ticked and the go-live log row filled.
