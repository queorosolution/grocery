# 00 — Decisions

Each decision has: **what**, **why**, **cost of the choice**.
If you change a decision, update this file first, then the docs that depend on it.

---

## D01 — Fully serverless (AWS + Cloudflare)

- **What:** Backend on AWS Lambda. Frontend on Cloudflare Pages. Images on Cloudflare R2. Database on Neon.
- **Why:** Low traffic at start, so pay per use is near zero. No servers to patch, which helps security.
- **Cost:** Cold starts (small with Go). Several free tiers to watch.
- **Oracle VM:** Not used in production (single VM, not serverless, idle reclaim risk). Optional for dev tools only.

## D02 — Modular monolith, four Lambda functions

- **What:** One Go codebase with clear modules. Four functions:
  - `api` — all HTTP routes.
  - `worker` — reads SQS events (notifications, stats, cart price sync).
  - `timer` — runs one-time timers (review, revision, payment timeouts).
  - `sync` — catalog connector runs.
- **Why:** One developer, one coding agent. Fewer deploy units, less duplicated wiring, fewer cold starts than one function per route.
- **Cost:** The `api` function is bigger. Fine at this size.
- The **cart service stays a separate service** (already built).

## D03 — Go on Lambda with AWS SAM

- **What:** `provided.al2023`, `arm64`, binary named `bootstrap`, `CGO_ENABLED=0`. Same as the cart service.
- **Why:** Fast start, low memory, strong types. You already know this setup.

## D04 — API Gateway **HTTP API** (not REST API)

- **What:** HTTP API with a custom domain `api.<PLATFORM_DOMAIN>`.
- **Why:** Cheaper and simpler. We do not need usage plans here (the cart service has them).
- **Cost:** No built-in per-key quotas. We use route throttling, Cloudflare rules and app-level limits instead.

## D05 — Neon Postgres (not RDS, not Aurora DSQL)

- **What:** Neon free plan, pooled connection string, `pgx` driver.
- **Why:**
  - RDS has no always-free tier for new AWS accounts and needs a VPC (NAT cost or a public DB).
  - Aurora DSQL is always free but is missing Postgres features we use (extensions such as `pg_trgm`, foreign keys).
  - Neon is real Postgres, permanently free at small size, scales to zero, no VPC.
- **Cost:** Free plan has a compute-hour cap and 0.5 GB storage per project. So we **never wake the database on a fixed short interval** (see D16).
- **Upgrade path:** Neon paid usage plan. No code change.

## D06 — Next.js, static export, client pages

- **What:** Next.js App Router with `output: "export"`, hosted as static files on Cloudflare Pages. Interactive pages are `"use client"`. One app with two areas: customer (`/`) and console (`/console/...`) for admin, shop and driver.
- **Why:**
  - You already know Next.js, so you can review and fix what the coding agent writes.
  - Static export = no Next server to run or pay for. All logic stays in the Go API (one place for rules and security).
  - Free hosting on Cloudflare Pages; fast from the edge.
- **How dynamic pages work** (`08-frontend-design.md` §11):
  - Shop pages `/s/[slug]` are **pre-built** for every shop, so shared links show the shop name and logo. A rebuild is started by a webhook when a shop is added (D30).
  - Private pages with ids (`/order/[id]`, console pages) use one placeholder page + a Cloudflare rewrite rule.
- **Not used:** Server Actions, Route Handlers, middleware, ISR, `next/image` optimisation (all need a Next server).
- **Cost of the choice:** a short rebuild (1–3 min) after a new shop is added; inline-script hashes must be added to the CSP after each build (script provided).
- **Rejected:** Next.js with a server (Vercel Pro ≈ USD 20/month, or OpenNext on Cloudflare Workers — more moving parts and size limits). Can be adopted later without rewriting pages, because pages are already client components.

## D07 — Google sign-in, our own session cookies

- **What:** Browser gets a Google ID token. The API verifies it and sets **its own** session: a short access token and a rotating refresh token, both in `HttpOnly; Secure; SameSite=Lax` cookies.
- **Why:** Tokens are never readable by JavaScript, so an XSS bug cannot steal them.
- **Super admin:** also needs a **TOTP code** (authenticator app). Staff, shop owners and drivers are **invited by email** and sign in with Google.

## D08 — Cart service does carts only; core owns bills, orders and payments

- **What:**
  - The browser adds and removes lines in the cart service with the shop's **public key**.
  - On bill submit, the core API reads the cart with the **platform key**, **re-prices every line from Neon**, and writes its own order.
  - The core keeps the cart service price list up to date (worker job).
- **Why:** The cart stays generic and reusable. The grocery rules (shop review, revision, slots, coupons, payment) live in one place.
- **Cost:** Two price copies. Neon is the **only source of truth**. The cart copy is for display only.

## D09 — Confirm-then-pay

- **What:** Bill submitted → shop checks by hand → (optional revision accepted by customer) → customer pays.
- **Why:** Shop fixes missing items before money moves. Fewer refunds.

## D10 — Pricing: markup on shop price, capped at MRP

- **What:**
  - `online_price = min(round_rupee(shop_price × (1 + markup)), mrp)`
  - `platform_margin = online_price − shop_price`
  - If the order's total margin is below `min_platform_margin`, add a **service fee** for the difference.
- **Why:** Customer never pays above MRP. Shop gets its normal price.
- Markup is stored in **basis points** (`250` = 2.5%), per shop and optionally per category.

## D11 — Payment providers behind one interface

- **What:** `PaymentProvider` with `PAYHERE` and `MANUAL` (bank transfer, admin marks paid).
- PayHere account owner per shop: `PLATFORM` (your account, default) or `SHOP` (shop's own account, later).
- Account state: `NOT_CONNECTED` → `SANDBOX` → `LIVE`. Shops with no usable payment method cannot take orders.
- **Why:** You have no PayHere account yet. Build and test on sandbox now, switch to live by config.

## D12 — Delivery type: `SLOT` now, `COURIER` later

- **What:** `DeliveryMethod` interface. `SLOT` is built. `COURIER` is a stub that returns "not available".

## D13 — Delivery fee rules from the admin panel

- **What:** base fee, per km, free km, per kg, free kg, free above order value, max distance, min and max fee.
- Distance = straight-line (haversine) × `1.3` road factor. **No Google distance API cost.**
- Weight = sum of item `weight_g × qty` (category default when missing).

## D14 — No stock reservation in phase 1

- **What:** We show `available` from the shop's data. The shop's **manual review** confirms real stock.
- **Why:** Krishna Super's stock lives in its own system. We cannot write to it. The manual review step already covers this.

## D15 — Catalog connector: pull sync, not live queries

- **What:** Admin adds the shop's Neon connection string (stored encrypted), maps columns, previews, then the `sync` function copies changed rows into our database.
- Triggers:
  1. A safety schedule every `interval_min` (default **60**) **only during shop open hours**.
  2. **Instant:** Krishna Super's own admin panel calls our signed trigger URL after a price or stock change.
  3. "Sync now" button.
- **Why:** Live queries on every page view are slow, break when their DB sleeps, and are a security risk.
- **Result:** Changes show on our site within seconds (trigger) or within the hour (schedule).
- **Neon care:** every run wakes both databases (ours and the shop's), so the schedule is hourly and the trigger does the fast work.

## D16 — One-time timers, no per-minute sweeper

- **What:** Each timeout is a one-time **EventBridge Scheduler** schedule that calls the `timer` function once (deleted after it runs).
- **Why:** A job every minute would keep Neon awake all day and use up the free compute hours.

## D17 — Events: outbox + SQS

- **What:** State changes write an `outbox` row **in the same transaction**. After commit, the API sends the event to SQS and marks it sent. A small sweep (every 15 min, open hours only) re-sends anything left unsent.
- **Why:** No lost notifications, no extra always-on polling.

## D18 — Notifications are platform-owned

- **What:** One SMS account and one WhatsApp account owned by the platform. Each shop has channel flags: `SMS`, `WHATSAPP`, `WA_LINK` (free, manual tap). No flag selected = off.
- Platform pays SMS. Usage is counted per shop. Daily caps protect the balance.
- Phone OTP is always SMS.

## D19 — Enums: one JSON file, generated code

- **What:** `contracts/enums.json` → `make gen` → Go constants + TypeScript unions. Postgres uses `TEXT` + `CHECK`, never native `ENUM`.
- **Why:** No mismatch between backend, frontend and database. Easy to add values.

## D20 — Permissions: role → permission map + scope

- **What:** Code asks `can(actor, "order.review", shopID)`. One map in `internal/shared/authz` says which role has which permission. The scope check (`shop_id`) is always part of the call.
- **Why:** No role-name checks spread across handlers. Same idea as Queoro entitlements.

## D21 — Analytics from a daily stats table

- **What:** The worker updates `shop_daily_stats` on order events. Dashboards read only that table plus small indexed queries.

## D22 — Images: browser resize, R2 presigned upload

- **What:** Browser resizes to max 1200 px WebP, asks the API for a presigned PUT URL (type and size locked), uploads to R2. Images are served from `img.<domain>`.

## D23 — Cancel rules

| Who | When | Result |
|---|---|---|
| Customer | Before payment | Allowed. Slot and coupon released. |
| Customer | After payment | Not allowed. "Report a problem" instead. |
| Shop | After payment, before `PACKED` | **Request** only, with a reason. Admin approves → refund record. |
| Shop | Item missing while picking | Partial refund request for that line. Admin approves. |
| Admin | Before payment: any time. After payment: `PAID`, `PICKING`, `PACKED`, `DELIVERY_FAILED` (while out for delivery, the driver reports failed first) | Allowed with reason. Refund record created if paid. |

## D24 — Phone verified at first order

- **What:** Sign-up asks for name (from Google), phone and address. The phone is **verified by SMS OTP when submitting the first bill**, not at sign-up.

## D25 — Order tracking by polling

- **What:** Order page polls every 20 s while the tab is visible and the order is active. Stops at final states. No WebSockets in phase 1.

## D26 — Cloudflare in front of the API

- **What:** `api.<domain>` is proxied through Cloudflare. Public catalog `GET`s send `Cache-Control: public, max-age=60` and a Cache Rule caches them.
- **Why:** Fewer Lambda calls and fewer Neon wake-ups. Free WAF basics and rate limiting rule.

## D27 — No night orders

- **What:** Orders only when the shop's `accepting_orders` is on **and** now is inside the shop's open hours. Checked on the server.

## D28 — Whole-number quantities in phase 1

- **What:** Customers order packs (`qty` = 1, 2, 3 …). `unit` + `size_label` describe the pack (e.g. `KG`, `5kg`).
- **Why:** Krishna Super does not sell loose fresh items yet. Whole numbers remove rounding and re-weighing problems.
- **Later:** Loose weighed items (final price after weighing) in phase 3. The schema already allows decimals.

## D29 — Server decides which buttons show

- **What:** Every order response has `allowedActions`, computed on the server from the transition table and the viewer.
- **Why:** The frontend never copies business rules. Less duplicated logic, fewer bugs when rules change.

## D30 — Rebuild webhook for shop pages

- **What:** When the admin creates a shop, or changes a shop's slug, name, logo or status, the API writes a `site.rebuild` event. The worker calls the **Cloudflare Pages deploy hook** (a secret URL in SSM). Several changes within 2 minutes cause **one** rebuild.
- **Why:** Shop pages are pre-built for link previews. Prices and products are always loaded live from the API, so **price and stock changes never need a rebuild**.
- **Safety net:** until the new build is live, `not-found.tsx` renders the shop screen for `/s/<new-slug>`.
