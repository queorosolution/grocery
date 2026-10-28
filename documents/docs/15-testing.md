# 15 — Testing

## 1. Test layers

| Layer | What | Tools | Runs |
|---|---|---|---|
| Unit (pure) | `billing`, `pricing`, `delivery/fee`, `money`, `geo`, `validate`, `crypt`, `ids`, templates | `go test` | every save / PR |
| Integration | services + repos on a **real Postgres** (fresh schema per package via `test/testdb`) | `go test` + Postgres service | PR |
| API | router with `httptest`, real DB, fake cart / PayHere / SMS | `go test` | PR |
| Security | `test/security/*` (see `11` §2) + `contracts/schema_test.sql` | `go test`, `psql` | PR |
| Contracts | `make check-contracts` | Go CLI | PR |
| Web unit | `lib/*`, hooks, key components | Vitest + Testing Library | PR |
| E2E smoke | 5 flows in a browser against `dev` | Playwright | after deploy to dev |

**No mocks for the database.** Fakes only for outside systems (`test/fakes`).

## 2. Must-pass tests (by requirement)

| Test | Covers |
|---|---|
| `TestOnlinePrice_*` (markup, rounding, MRP cap, MRP below shop price) | FR-CAT-03, D10 |
| `TestFee_*` (free km, per km ceil, per kg, free above, min / max, out of range) | FR-FEE-01 |
| `TestBuild_*` (unavailable line excluded, qty limits, service fee top-up, coupon caps, min order) | FR-BILL-03, FR-CPN-04 |
| `TestSubmit_HappyPath` | FR-BILL-02 |
| `TestSubmit_Checks` (each error code once) | FR-BILL-03 |
| `TestSubmit_Idempotent` | BIZ-05 |
| `TestSlot_Race` (20 parallel, capacity 5) | FR-SLOT-02, BIZ-04 |
| `TestReview_ApproveFreezes`, `TestReview_Revise_ThenAccept`, `TestReview_Reject_ReleasesSlot` | FR-BILL-05..07 |
| `TestTransition_Table` (every row of `order_transitions` allowed for listed actors, refused for all others) | `03-order-flow.md` |
| `TestTransition_StaleVersion` | FR-BILL-09 |
| `TestTimer_NoOpWhenMoved` | §5 of `03` |
| `TestCoupon_Race`, `TestCoupon_PerUser`, `TestCoupon_ReleasedOnExpire` | FR-CPN-03 |
| `TestPayHere_Notify_*` (success, bad sig, wrong amount, replay, late, card fields stripped, chargeback) | FR-PAY-04..06 |
| `TestLedger_PaidAndDelivered_SumsToTotal` | `05` §4 |
| `TestDeliveryCode_*` (only customer sees, 5 tries lock, admin unlock) | FR-DLV-03..04 |
| `TestConnector_Mapping_RejectsUnknownColumns`, `TestConnector_Holds`, `TestConnector_Unmatched`, `TestConnector_NoOverlap`, `TestConnector_SSRF` | FR-SYNC-* |
| `TestTrigger_Signature` | BIZ-16 |
| `TestAuthzMatrix` (generated) | AUTHZ-* |
| `TestRedaction` | DATA-01 |
| `TestTemplates_Max160` | FR-NTF-05 |
| `TestRoutes_MatchDocs` (every route in `06-api.md` is in `routes.go` with a guard) | API contract |

## 3. E2E smoke flows (Playwright, dev)

1. Guest → home → shop → add 3 items → checkout → sign in (test Google account) → bill shows server prices.
2. Customer submits → staff approves in console → customer pays in PayHere **sandbox** → order `PAID`.
3. Staff revises a bill → customer sees old vs new → accepts → pays.
4. Staff picks, packs, assigns driver, sends out → driver enters code from customer page → `DELIVERED`.
5. XSS seed product renders as text on shop, bill and console pages; no CSP errors in the console on any page (XSS-08).

Also on a Pages preview: `/order/<real id>` and `/console/shop/<real id>/order/<real id>` load through the rewrite and show the right record; a brand-new shop slug renders through `not-found.tsx` before the rebuild finishes.

## 4. Test data

- `make seed-local` / `test/testdb.Seed()` creates: 2 shops (A active, B active), 1 admin, owner + staff + driver for A, owner for B, 2 customers, 20 products, slots for 3 days, one fee rule, one coupon.
- Fixed UUIDs (like `contracts/schema_test.sql`) so tests read clearly.
- `clock.Set(t)` in tests to control "now" (open hours, cutoffs, timers).

## 5. Definition of done (every prompt)

- [ ] Code builds; `go vet`, `staticcheck`, lint, typecheck clean.
- [ ] New code has tests; listed must-pass tests for the touched area pass.
- [ ] Security IDs named in the prompt verified.
- [ ] No new dependency outside the allowed lists.
- [ ] No file over the size limit; no duplicated helper.
- [ ] Docs updated if behaviour changed (and `contracts/` regenerated if enums changed).
