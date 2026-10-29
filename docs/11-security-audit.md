# 11 — Security audit and vulnerability checks

Use this file three ways:
1. **While building:** each prompt in `PROMPTS.md` names the check IDs it must satisfy.
2. **In CI:** the automated checks in §3 run on every pull request.
3. **Before go-live:** walk §4 by hand on the `dev` stack and tick every line.

Severity: **C** critical · **H** high · **M** medium. Every **C** and **H** must have an automated test.

Mapped to OWASP Top 10 (2021) and OWASP API Security Top 10 (2023) in the last column.

---

## 1. Checklist

### 1.1 Cross-site scripting (XSS)

| ID | Risk | Where | Control | How to verify | Sev | OWASP |
|---|---|---|---|---|---|---|
| XSS-01 | Script in product name / brand from the shop DB or admin | Shop page, bill, console | React text rendering; no `dangerouslySetInnerHTML`; CSP without `unsafe-inline` scripts | Seed product `<img src=x onerror=alert(1)>`; page shows the text, no alert; ESLint `react/no-danger` = error | H | A03 |
| XSS-02 | Script in search `q` reflected | `/search` | Same as XSS-01; `q` shown as text | Visit `/search?q=<script>alert(1)</script>` → text only | H | A03 |
| XSS-03 | Open redirect / DOM XSS through `next` param | Sign-in return | `safeNext()` allows only `/path`, not `//`, not `javascript:` | Unit tests for `//evil.com`, `/\evil.com`, `javascript:alert(1)`, `https://evil.com` → `/` | H | A01 |
| XSS-04 | `javascript:` or `data:` URLs in image / logo fields | Product, shop | API accepts only `https://img.<domain>/…` for uploaded images; `safeUrl` in UI | API test: `imageUrl: "javascript:alert(1)"` → `400` | H | A03 |
| XSS-05 | Forged payment form action | Pay button | `isAllowedPayHereAction` (exact two URLs); form built with DOM API | Unit test with `https://evil.com/pay` → refused | H | A08 |
| XSS-06 | Text in notes shown to staff | Console order | Text rendering; note max 300 chars; control chars stripped | Seed note with HTML → shown as text | M | A03 |
| XSS-08 | CSP weakened to make Next.js inline scripts run | Post-build hashes per page; build fails on an unhashed inline script; no `'unsafe-inline'` in `script-src` | CI: run `csp-hashes.mjs --check`; deployed preview shows no CSP errors on every page | H | A05 |
| XSS-09 | Secret leaked through a `NEXT_PUBLIC_` variable | Only the variables in `13-config.md` §3 may exist; CI fails on any other `NEXT_PUBLIC_` name | CI grep check | H | A05 |
| XSS-07 | Uploaded SVG / HTML served as image | `img.` domain | Only `image/jpeg`, `image/png`, `image/webp` presigned; separate cookie-less domain; `nosniff` header on `img.` | Presign with `image/svg+xml` → `400 UPLOAD_REJECTED` | M | A05 |

### 1.2 Cross-site request forgery (CSRF) and CORS

| ID | Risk | Control | How to verify | Sev | OWASP |
|---|---|---|---|---|---|
| CSRF-01 | Other site posts to our API with the user's cookies | `SameSite=Lax` + required `X-Requested-With: gp-web` + `Origin` allow-list on non-GET | `curl -X POST` without header → `403`; with `Origin: https://evil.com` → `403` | H | A01 |
| CSRF-02 | Login CSRF on `/auth/google` | Same header + origin rule | Same test on `/auth/google` | M | A07 |
| CORS-01 | Wildcard or reflected origin with credentials | Exact match list `ALLOWED_ORIGINS`, `Vary: Origin` | Request with `Origin: https://evil.com` → no `Access-Control-Allow-Origin` | H | A05 |
| CORS-02 | `null` origin accepted | `null` never allowed | Test `Origin: null` | M | A05 |

### 1.3 Server-side request forgery (SSRF)

| ID | Risk | Control | How to verify | Sev | OWASP |
|---|---|---|---|---|---|
| SSRF-01 | Connector string points to internal hosts | Resolve host → refuse loopback, private (10/8, 172.16/12, 192.168/16, 100.64/10), link-local (169.254/16, fe80::/10), unique-local (fc00::/7), `0.0.0.0`, metadata IPs; connect to the **resolved IP** (prevents DNS rebinding) with TLS server name set | Table test: `localhost`, `127.0.0.1`, `[::1]`, `169.254.169.254`, `10.0.0.5`, `2130706433`, `0x7f000001`, a name resolving to 127.0.0.1 → all `CONNECTOR_TEST_FAILED` | C | A10 |
| SSRF-02 | Non-Postgres port scanning via error messages | Only port 5432 allowed; errors returned as one generic message; details only in logs | Test with port 22 → generic error | H | A10 |
| SSRF-03 | Server fetching user URLs | The server never fetches user-given URLs (images are uploaded, not fetched) | Code review: no `http.Get` on user input | H | A10 |

### 1.4 Injection

| ID | Risk | Control | How to verify | Sev | OWASP |
|---|---|---|---|---|---|
| SQLI-01 | SQL injection in any query | `pgx` parameters only; `gosec` G201/G202 | `gosec` clean; grep for `fmt.Sprintf(` near `Query` → none | C | A03 |
| SQLI-02 | Injection through connector table / column names | Names must exist in `information_schema` list + regex `^[a-z_][a-z0-9_]{0,62}$` + `pgx.Identifier.Sanitize()` | Mapping with `price"; DROP TABLE x;--` → `400` | C | A03 |
| SQLI-03 | Search text breaking trigram query | Parameter only; length 2–60 | Search `%' OR 1=1 --` → normal empty result | H | A03 |
| HDR-01 | Header / SMS injection (newlines) | Strip control characters from template variables and names | Name with `\r\n` → stored without them | M | A03 |
| LOG-01 | Log injection | `slog` JSON logs (values escaped) | Value with newline logged as one JSON line | M | A09 |

### 1.5 Authentication

| ID | Risk | Control | How to verify | Sev | OWASP |
|---|---|---|---|---|---|
| AUTHN-01 | Google token for another app accepted | `aud == GOOGLE_CLIENT_ID`, `iss`, `exp`, `email_verified` | Tests with wrong `aud`, expired, unverified email → `401` | C | API2 |
| AUTHN-02 | JWT algorithm confusion (`none`, RS→HS) | Parser allows **only** HS256; checks `iss`, `aud`, `exp` | Tokens with `alg: none` and a different alg → `401` | C | API2 |
| AUTHN-03 | Stolen refresh token reused | Rotation + family revoke on reuse + alarm | Use an old refresh token → `401`, all family sessions revoked | H | API2 |
| AUTHN-04 | Cookies readable by JS or sent over HTTP | `HttpOnly; Secure; SameSite=Lax`; refresh cookie path `/v1/auth` | Inspect `Set-Cookie` in test | H | A07 |
| AUTHN-05 | Admin access without second factor | Admin guard needs `totp` claim from a TOTP-verified session | Admin route with a valid non-TOTP session → `401 TOTP_REQUIRED` | C | API5 |
| AUTHN-06 | TOTP code replay / brute force | `last_used_step`; 5 tries / 15 min | Same code twice → second fails; 6th try → `429` | H | A07 |
| AUTHN-07 | Invite hijack | Hashed single-use token; Google email must equal invite email; 7-day expiry | Accept with other email → `403`; second accept → `404` | H | A07 |
| AUTHN-08 | Blocked user keeps access | Block revokes sessions; refresh checks `users.status` | Block user → next refresh `401` | M | A07 |
| AUTHN-09 | Phone OTP brute force | 4–6 digit code, 5 tries, 10 min expiry, HMAC stored | 6th try → `OTP_LOCKED` | H | A07 |

### 1.6 Authorization (BOLA / IDOR, privilege)

| ID | Risk | Control | How to verify | Sev | OWASP |
|---|---|---|---|---|---|
| AUTHZ-01 | Customer reads / changes another customer's order | Query filter `customer_id = me` + RLS | Customer B calls `GET /orders/{A's id}` → `404`; cancel → `404` | C | API1 |
| AUTHZ-02 | Shop A member uses `/console/shops/{B}` | Membership check for path shop + RLS `app.shop_id` | Owner A on shop B routes → `404` for **every** console route (generated test) | C | API1 |
| AUTHZ-03 | Driver completes someone else's delivery | `driver_id = me` + RLS | Driver X completes driver Y's order → `404` | C | API1 |
| AUTHZ-04 | Staff manages team or becomes owner | `team.manage` only for owners; owners cannot invite owners; cannot edit own role; cannot remove last owner | Staff `POST /team/invites` → `403`; owner invites `SHOP_OWNER` → `400` | H | API5 |
| AUTHZ-05 | Mass assignment (status, price, shopId, role in body) | Per-endpoint request structs + `DisallowUnknownFields` | Send `status`, `totalCents`, `role` fields → `400 VALIDATION_ERROR` | H | API3 |
| AUTHZ-06 | Customer-only fields leaked to console / driver (or reverse) | View mappers per viewer (`order/view.go`) | Driver view has no `deliveryCode` and no line prices; customer view has no other customers' data | H | API3 |
| AUTHZ-07 | Admin routes reachable by non-admin | `admin` guard; route table test | Every `/admin/*` route with an owner session → `403` (generated test) | C | API5 |
| AUTHZ-08 | Connector-managed fields edited by shop | Server refuses those fields | PATCH price on connector product → `400` | M | API3 |
| AUTHZ-09 | RLS bypass if a query forgets a filter | RLS on all private tables; app role is `app_rw` | `contracts/schema_test.sql` passes (30/30) in CI | C | A01 |

### 1.7 Business logic and money

| ID | Risk | Control | How to verify | Sev | OWASP |
|---|---|---|---|---|---|
| BIZ-01 | Price / total tampering | Bill built only by `billing.Build` from DB prices | Change cart price in cart service → bill still uses DB price | C | API6 |
| BIZ-02 | Negative, zero or huge quantity | `qty` integer 1..`max_per_order`; ≤ `max_lines` | `qty: -1`, `0`, `1000000`, `1.5` → `400` / `QTY_LIMIT` | H | API6 |
| BIZ-03 | Coupon race beyond limit | Atomic `UPDATE … WHERE used_count < usage_limit`; per-user count in the same transaction | 20 parallel applies on a limit-5 coupon → exactly 5 succeed | H | API6 |
| BIZ-04 | Slot overbooking | Atomic booking query | 20 parallel submits on capacity 5 → exactly 5 orders | H | API6 |
| BIZ-05 | Double submit / double charge | `Idempotency-Key`; unique payment id per attempt | Same key twice → same order id; different body same key → `409` | H | API6 |
| BIZ-06 | Customer edits PayHere form amount | Hash covers amount; notify checks amount equals ours | Notify with different `payhere_amount` → not paid, logged | C | API6 |
| BIZ-07 | Forged notify | `md5sig` check (constant time) | Notify with bad `md5sig` → `400`, order unchanged | C | API2 |
| BIZ-08 | Replayed notify | Unique `(provider, provider_ref)` | Same notify twice → one payment row, one ledger set | H | API6 |
| BIZ-09 | Paid after expiry | Late payment → refund request | Expire order, then notify success → refund `LATE_PAYMENT` created, order stays `EXPIRED` | H | API6 |
| BIZ-10 | Cancel after payment through any route | Transition table; customer cancel only before `PAID` | Customer cancel on `PAID` → `409 INVALID_TRANSITION` | H | API6 |
| BIZ-11 | Delivery code guessing | 5 tries → lock; 4 digits | 6th try → `DELIVERY_CODE_LOCKED` | H | API6 |
| BIZ-12 | Fake order flooding | Phone verified, ≤ 3 open unpaid, 10 bills / hour | 4th open order → `TOO_MANY_OPEN_ORDERS` | M | API4 |
| BIZ-13 | SMS pumping | Turnstile, per-user / per-phone caps, `+94` only, global cap | Send to `+1…` → `400`; 4th send in an hour → `429` | H | API4 |
| BIZ-14 | Coupon makes platform lose money | Cap by platform income unless `allow_loss` | Unit test in `billing` | M | API6 |
| BIZ-15 | Poisoned prices from shop DB | Holds for > MRP, > 50% drop, zero | Sync row with price 0 → `PRICE_HOLD`, old price kept | H | A08 |
| BIZ-16 | Trigger URL abuse | HMAC + 5-min window + rate limit | Old timestamp → `401`; wrong HMAC → `401`; 31st call / hour → `429` | M | API4 |
| BIZ-17 | Stale order overwrite | `version` check | Two reviews with the same version → second `409 STALE_ORDER` | M | API6 |
| BIZ-18 | Orders when shop closed | Server checks hours + switch | Submit outside hours → `SHOP_CLOSED` | M | API6 |

### 1.8 Data exposure and privacy

| ID | Risk | Control | How to verify | Sev | OWASP |
|---|---|---|---|---|---|
| DATA-01 | PII / tokens in logs | `logx` redaction | Test: log a struct with phone, token → output masked | H | A09 |
| DATA-02 | Secrets committed | `gitleaks` in CI; `.env` in `.gitignore` | CI job | C | A05 |
| DATA-03 | Card data stored | Notify sanitiser drops card fields | Test: notify with `card_no` → `payments.raw` has no card fields | C | A02 |
| DATA-04 | Delivery code visible to driver / shop | View mapper | Driver and console views never include `deliveryCode` | H | API3 |
| DATA-05 | Account / shop enumeration | Generic auth errors; `404` for out-of-scope | Compare responses for existing vs non-existing other-shop ids → identical | M | API1 |
| DATA-06 | Lost data | Neon PITR on; restore drill | Done once before go-live, noted in `14-deployment.md` | H | — |
| DATA-07 | Deleted account data remains | `DELETE /me` anonymises | Test: after delete, no phone / email / addresses | M | — |
| DATA-08 | Connection string returned by API | Write-only; masked display | `GET` connector → no password anywhere in JSON | C | API3 |
| DATA-09 | Exact customer location in cache | Home `lat/lng` rounded to 2 decimals by client and server | Server rounds again before cache key | M | — |

### 1.9 Infrastructure and configuration

| ID | Risk | Control | How to verify | Sev | OWASP |
|---|---|---|---|---|---|
| INFRA-01 | Over-wide IAM | Per-function least privilege, no `*` actions | `cfn-lint` + review of `template.yaml`; `checkov` optional | H | A05 |
| INFRA-02 | Missing security headers / weak CSP | `_headers` + `httpx` headers | securityheaders.com grade A on `www` and `api`; no CSP console errors | M | A05 |
| INFRA-03 | Plain HTTP | Cloudflare "Always use HTTPS" + HSTS | `curl http://…` → 301 to https | M | A02 |
| INFRA-04 | DoS / cost spike | Throttle, body limits, timeouts, reserved concurrency, budget alert | Load test 100 rps for 1 min on `dev` → `429`s, no errors, cost alarm quiet | M | API4 |
| INFRA-05 | App uses DB owner (RLS skipped) | `DATABASE_URL` user is `app_rw` | Start-up check: `SELECT current_user` must be `app_rw`, else refuse to start | C | A01 |
| INFRA-06 | Secrets in Lambda env | Env holds only SSM names | Review `template.yaml`; test fails if an env value looks like a key | H | A05 |
| INFRA-07 | Verbose errors | `INTERNAL` with request id only | Force a panic in dev → generic message | M | A05 |
| INFRA-08 | Presigned URL misuse | Type + length locked, 5 min, server key name | Upload a bigger file with the URL → R2 refuses | M | A05 |

### 1.10 Supply chain

| ID | Risk | Control | How to verify | Sev | OWASP |
|---|---|---|---|---|---|
| SUP-01 | Vulnerable Go modules | `govulncheck` in CI | CI job | H | A06 |
| SUP-02 | Vulnerable npm packages | `npm audit --audit-level=high`, lockfile committed | CI job | H | A06 |
| SUP-03 | Compromised GitHub Actions | Actions pinned to commit SHA; minimal `permissions:` | Review workflows | M | A08 |
| SUP-04 | Unexpected new dependency | Allowed list in `07` / `08`; CI diff check on `go.mod` / `package.json` | PR review | M | A06 |

---

## 2. Security test files

The checks that walk the whole API or the whole source tree live in `backend/test/security/`. The checks of one module live next to that module, because they need its fixtures (a shop, a product, a cart, a payment); section 2.1 says where.

| File | Contains |
|---|---|
| `backend/test/security/authz_matrix_test.go` | Generated from `routes.go`: every `perm:` and `admin` route × every role × own / other shop → expected status (AUTHZ-02, AUTHZ-07) |
| `backend/test/security/authz_routes_test.go` | The other routes: public, user and signed; unknown body fields on every route (AUTHZ-05) |
| `backend/test/security/authz_owner_test.go` | A stranger on every route that names an order or an address, compared with an id that does not exist (AUTHZ-01, DATA-05); a driver on another driver's order (AUTHZ-03) |
| `backend/test/security/static_test.go` | SQL text is always fixed (SQLI-01); only listed packages make HTTP requests (SSRF-03); `source_test.go` holds the source reader they share |
| `backend/test/security/template_test.go` | No wildcard IAM action or resource (INFRA-01); no secret in Lambda variables or stack parameters (INFRA-06) |
| `backend/test/security/ci_test.go` | The CI steps of section 3 are still there (SUP-01, SUP-02, DATA-02, AUTHZ-09, XSS-08); actions pinned (SUP-03); web lint rules and the `NEXT_PUBLIC_` check wired (XSS-08, XSS-09) |
| `backend/test/security/coverage_test.go` | Every C and H row of section 1 has a test, or is a listed gap |
| `backend/internal/order/pay_notify_test.go` | PayHere notify: bad sig, wrong amount, replay, late payment, card field stripping (BIZ-06..09, DATA-03) |
| `backend/internal/order/submit_limits_test.go`, `coupon_release_test.go` | Slot and coupon races, 20 goroutines (BIZ-03, BIZ-04) |
| `backend/internal/connector/source/guard_test.go` | Connector host table (SSRF-01, SSRF-02) |
| `backend/internal/auth/*_test.go` | Google token cases, JWT alg cases, refresh reuse, TOTP (AUTHN-01..06) |
| `backend/internal/shared/logx/logx_test.go` | Redaction (DATA-01) |
| `contracts/schema_test.sql` | RLS and constraints (30 checks), run by `make test-schema` in CI (AUTHZ-09) |
| `web/src/lib/safeUrl.test.ts` | `safeNext`, `safeUrl`, `isAllowedPayHereAction` (XSS-03..05) |
| `web/e2e/xss.spec.ts` | XSS-01, XSS-02 in a real browser (P34) |

### 2.1 Where each C and H check is tested

Written by name: a test belongs to a check when its file names the check ID. `TestCoverage_EveryCriticalAndHighCheckHasATest` reads section 1, fails for a C or H row without a test, and fails again when a gap listed in the test is closed but not removed. A new test only has to carry the ID in a comment or a test name.

| ID | Sev | Tested in |
|---|---|---|
| XSS-01 | H | open: the browser check with a seeded product (E2E, P34); the lint rule is kept on by `TestWeb_SecurityControlsAreWired` |
| XSS-02 | H | open: needs the `/search` page (P29) |
| XSS-03 | H | `web/lib/safeUrl.test.ts` |
| XSS-04 | H | `catalog/product_test.go`, `web/lib/safeUrl.test.ts` |
| XSS-05 | H | `web/lib/safeUrl.test.ts` |
| XSS-08 | H | `test/security/ci_test.go` |
| XSS-09 | H | `test/security/ci_test.go` |
| CSRF-01 | H | `http/router_test.go`, `shared/httpx/middleware_test.go` |
| CORS-01 | H | `http/router_test.go`, `shared/httpx/middleware_test.go` |
| SSRF-01 | C | `connector/connector_test.go`, `connector/source/guard_test.go` |
| SSRF-02 | H | `connector/connector_test.go`, `connector/source/guard_test.go`, `connector/source/postgres_test.go` |
| SSRF-03 | H | `test/security/static_test.go` |
| SQLI-01 | C | `test/security/ci_test.go`, `test/security/static_test.go` |
| SQLI-02 | C | `connector/mapping_test.go`, `connector/source/postgres_test.go`, `test/security/static_test.go` |
| SQLI-03 | H | `catalog/search_test.go` |
| AUTHN-01 | C | `auth/google_test.go` |
| AUTHN-02 | C | `auth/token_test.go` |
| AUTHN-03 | H | `auth/session_test.go` |
| AUTHN-04 | H | `auth/signin_test.go` |
| AUTHN-05 | C | `auth/totp_test.go` |
| AUTHN-06 | H | `auth/totp_test.go` |
| AUTHN-07 | H | `team/invite_test.go` |
| AUTHN-09 | H | `user/phone_test.go` |
| AUTHZ-01 | C | `order/orders_test.go`, `test/security/authz_routes_test.go` |
| AUTHZ-02 | C | `order/console_test.go`, `team/team_test.go`, `test/security/authz_matrix_test.go` |
| AUTHZ-03 | C | `order/delivery_code_test.go`, `test/security/authz_routes_test.go` |
| AUTHZ-04 | H | `team/team_test.go` |
| AUTHZ-05 | H | `test/security/authz_routes_test.go` |
| AUTHZ-06 | H | `order/view_test.go` |
| AUTHZ-07 | C | `test/security/authz_matrix_test.go` |
| AUTHZ-09 | C | `test/security/ci_test.go` |
| BIZ-01 | C | `billing/build_test.go`, `order/quote_test.go` |
| BIZ-02 | H | `billing/build_test.go`, `order/quote_test.go` |
| BIZ-03 | H | `order/coupon_release_test.go`, `test/security/ci_test.go` |
| BIZ-04 | H | `order/submit_limits_test.go`, `test/security/ci_test.go` |
| BIZ-05 | H | `order/submit_limits_test.go` |
| BIZ-06 | C | `order/pay_notify_test.go` |
| BIZ-07 | C | `order/pay_notify_test.go` |
| BIZ-08 | H | `order/pay_notify_test.go` |
| BIZ-09 | H | `order/pay_notify_test.go` |
| BIZ-10 | H | `order/cancel_test.go` |
| BIZ-11 | H | `order/delivery_code_test.go`, `order/transition_test.go` |
| BIZ-13 | H | `user/phone_test.go` |
| BIZ-15 | H | `connector/run_test.go` |
| DATA-01 | H | `shared/logx/logx_test.go`, `user/phone_test.go` |
| DATA-02 | C | `test/security/ci_test.go` |
| DATA-03 | C | `order/pay_notify_test.go` |
| DATA-04 | H | `order/delivery_code_test.go` |
| DATA-06 | H | manual: restore drill before go-live (P35, `14-deployment.md`) |
| DATA-08 | C | `connector/connector_test.go`, `shop/payment_test.go` |
| INFRA-01 | H | `test/security/ci_test.go`, `test/security/template_test.go` |
| INFRA-05 | C | `shared/db/open_test.go` |
| INFRA-06 | H | `test/security/template_test.go` |
| SUP-01 | H | `test/security/ci_test.go` |
| SUP-02 | H | `test/security/ci_test.go` |

## 3. Automated checks (CI, every pull request)

```bash
# Backend
cd backend
go vet ./...
staticcheck ./...
gosec -exclude-generated ./...
govulncheck ./...
go test -race ./...                       # includes test/security with a real Postgres service
psql "$TEST_DB" -f ../contracts/schema.sql && psql "$TEST_DB" -f ../contracts/schema_test.sql

# Contracts
make check-contracts                      # enums.json ↔ schema.sql ↔ generated code

# Frontend
cd ../web
npm ci
npm run lint                              # includes react/no-danger, no-eval, no-implied-eval
npm run typecheck
npm test
npm run build                             # next build + gen-redirects + csp-hashes (fails on unhashed inline script)
npm audit --audit-level=high

# Repo
gitleaks detect --no-banner
```

Failing any of these blocks the merge.

## 4. Before go-live (manual, on `dev`)

- [ ] OWASP ZAP **baseline** scan against `https://dev.<domain>` and `https://api-dev.<domain>` — no High alerts.
- [ ] securityheaders.com: `www` and `api` grade A.
- [ ] Try every row in §1 marked **C** by hand with `curl` and a second test account.
- [ ] Sign in as owner of shop A; change the URL to shop B's id on every console screen → nothing loads.
- [ ] PayHere sandbox: pay, cancel, fail, and replay the notify body with curl → behaviour as in BIZ-06..09.
- [ ] SMS: try 6 OTP sends in a row → limited; non-`+94` number → refused.
- [ ] Connector: test with `127.0.0.1` and `169.254.169.254` hosts → refused.
- [ ] Check CloudWatch logs for one full order flow: no phone numbers, codes, tokens or secrets.
- [ ] Neon restore drill done (branch from a past time, open, check rows).
- [ ] AWS Budget alert and all alarms in `10-security.md` §4.8 exist and fire a test.
- [ ] Privacy page and terms published; account deletion works.
- [ ] Super admin TOTP enabled; admin phone receives sign-in SMS.

Sign-off: date, who checked, commit hash → add to `14-deployment.md` go-live log.
