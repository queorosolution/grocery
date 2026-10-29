# 16 — Roadmap

Build for many shops, launch with one. Each phase ends with something running on `dev` with tests.

## Phase 0 — Foundations

- Repo, Makefile, contracts generator, migrations, `shared/*` helpers, router, middleware, SAM skeleton, CI.
- Web skeleton: router, providers, `ui/` primitives, `lib/api.ts`, tokens.
- **Done when:** `GET /v1/health` and `GET /v1/bootstrap` work on `dev`; CI green with security tools.

## Phase 1 — Krishna Super MVP (sandbox payments)

| Area | Scope |
|---|---|
| Auth | Google sign-in, sessions, onboarding, phone OTP, admin TOTP, invites |
| Admin | Shops, settings, hours, slots, fee rules, payment accounts (sandbox + manual), cart tenant, readiness |
| Catalog | Master products, shop products, pricing engine, **Neon connector + trigger**, sync issues |
| Customer | Home, 2-way search, shop page, cart, bill page, orders, buy-again |
| Orders | Submit, review / revise / reject, timers, coupon, payment, picking, packing, driver, delivery code |
| Money | Ledger, refunds (manual), payouts |
| Notify | SMS + WA link |
| Dashboards | Owner, staff, driver, admin |

**Done when:** the 5 E2E flows pass on `dev` and every **C** / **H** check in `11` passes.

## Phase 1.5 — Go live

- PayHere live, SMS sender ID, privacy pages, go-live checklist (`14` §7).
- `FEATURE_SINGLE_SHOP=true`.

## Phase 2 — More shops

- Second shop onboarding (manual by admin), single-shop mode off.
- Lowest-price section across shops, shop distance filter.
- CSV product import for shops without a database.
- WhatsApp Cloud API channel.
- Route grouping for drivers (orders per slot + zone, stop order).
- Owner payouts statement PDF.

## Phase 3 — Growth

- Shops with their **own** PayHere account (`owner_type = SHOP`), commission invoices.
- `COURIER` delivery type.
- Loose weighed items (price after weighing).
- Move the web app to a Next.js server (OpenNext on Cloudflare or Vercel) only if server rendering is needed for SEO of product pages.
- Sinhala and Tamil UI.
- Automatic refunds (if the PayHere account supports it).
- Shop self sign-up with admin approval.
