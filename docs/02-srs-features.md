# 02 — SRS: feature perspective

- IDs look like `FR-BILL-03`. Use them in commits and tests.
- **Priority:** **M** = must for phase 1 · **S** = should, phase 1 if time · **C** = later.
- Phase 1 = Krishna Super live with slot delivery, PayHere (sandbox → live) and manual payment.

## Feature map

| Code | Feature | Main module |
|---|---|---|
| F1 | Sign-in and accounts | `auth`, `user` |
| F2 | Addresses and location | `user` |
| F3 | Shops and settings | `shop` |
| F4 | Catalog and pricing | `catalog`, `pricing` |
| F5 | Shop database connector | `connector` |
| F6 | Home and search | `catalog` |
| F7 | Cart | cart service + web |
| F8 | Bill and shop review | `order` |
| F9 | Slots and delivery fee | `delivery` |
| F10 | Coupons | `coupon` |
| F11 | Payment | `payment` |
| F12 | Fulfilment and delivery | `order`, `delivery` |
| F13 | Cancels and refunds | `refund` |
| F14 | Notifications | `notify` |
| F15 | Dashboards and analytics | `stats` |
| F16 | Ledger and payouts | `finance` |
| F17 | Buy again | `catalog` |
| F18 | Admin, team and audit | `admin`, `team`, `audit` |

---

## F1 — Sign-in and accounts

| ID | Requirement | P |
|---|---|---|
| FR-AUTH-01 | Users sign in with Google only. The API verifies the Google ID token (signature, `iss`, `aud`, `exp`, `email_verified`). | M |
| FR-AUTH-02 | The API sets an access cookie (15 min) and a refresh cookie (30 days, rotated on every use, stored hashed). Both `HttpOnly; Secure; SameSite=Lax`, host `api.<domain>`. | M |
| FR-AUTH-03 | Refresh token reuse (an old token used again) revokes the whole session family. | M |
| FR-AUTH-04 | Super admin emails come from `SUPER_ADMIN_EMAILS`. Super admin must pass TOTP after Google before any admin route works. | M |
| FR-AUTH-05 | Shop owners, staff and drivers join by **invite** (email + role + shop). Invite link valid 7 days, single use, must match the Google email. | M |
| FR-AUTH-06 | New customer onboarding: name (pre-filled from Google), phone, first address. Max 3 screens. | M |
| FR-AUTH-07 | Phone is verified with an SMS OTP before the first bill is submitted. Sending a code needs a Cloudflare Turnstile check. | M |
| FR-AUTH-08 | Logout revokes the session on the server. "Log out everywhere" revokes all sessions. | S |
| FR-AUTH-09 | User can delete their account (soft delete only). Refused while an order is open (unpaid bills included), a refund is unpaid, the cart has items, or the user is a shop or platform member. Phone and addresses are removed; name and e-mail stay for history. | S |

## F2 — Addresses and location

| ID | Requirement | P |
|---|---|---|
| FR-ADDR-01 | Address = label, text line, landmark, map pin (lat/lng). Google Places autocomplete + draggable pin. | M |
| FR-ADDR-02 | On save, the API returns which active shops deliver to this pin (within each shop's `max_km`). | M |
| FR-ADDR-03 | Max 5 addresses per user. One default. | M |
| FR-ADDR-04 | Lat/lng limited to Sri Lanka bounding box. | M |

## F3 — Shops and settings

| ID | Requirement | P |
|---|---|---|
| FR-SHOP-01 | Only the super admin creates shops (no self sign-up). New shops start as `DRAFT` and not accepting orders. | M |
| FR-SHOP-02 | Shop settings: delivery type, payment mode, driver source, notify channels, markup (bp), min platform margin, min order, review / revision / payment timeouts. | M |
| FR-SHOP-03 | Weekly open hours + `accepting_orders` switch. Bill submit is refused outside open hours (`SHOP_CLOSED`). | M |
| FR-SHOP-04 | A shop can take orders only if: status `ACTIVE`, accepting, open now, has a usable payment method, has at least one future slot. | M |
| FR-SHOP-05 | Shop page shows name, logo, theme color, open/closed badge, next slot. | M |
| FR-SHOP-06 | Single-shop mode: when `FEATURE_SINGLE_SHOP=true` and one shop is active, home opens that shop directly. | S |

## F4 — Catalog and pricing

| ID | Requirement | P |
|---|---|---|
| FR-CAT-01 | Master products: name, brand, size label, unit (`PCS`, `KG`, `G`, `L`, `ML`), `weight_g`, barcode (unique when present), category, image, keywords (Sinhala / Tamil / English). | M |
| FR-CAT-02 | Shop products link a shop to a master product with: MRP, shop price, online price, stock, available, max per order, managed by (`MANUAL` / `CONNECTOR`). | M |
| FR-CAT-03 | `online_price` is recalculated **in the same transaction** every time shop price, MRP or markup changes. | M |
| FR-CAT-04 | Every price change writes a `price_changes` row (old, new, source, who). | M |
| FR-CAT-05 | Fields managed by a connector are read-only in the shop console. | M |
| FR-CAT-06 | Changes to online price or availability publish `catalog.changed` so the cart service price list is updated. | M |

## F5 — Shop database connector

| ID | Requirement | P |
|---|---|---|
| FR-SYNC-01 | Kind supported: `NEON_POSTGRES` (any Postgres with TLS). | M |
| FR-SYNC-02 | Connection string stored only in SSM as `SecureString`. Never returned by any API. Shown as `postgres://user:•••@host/db`. | M |
| FR-SYNC-03 | "Test connection" checks TLS, login, and that the user is **read only** (warn if it can write). | M |
| FR-SYNC-04 | Admin picks a table or view from a list read from `information_schema`. No free SQL text. | M |
| FR-SYNC-05 | Mapping: source columns → `external_id` (required), `barcode`, `name`, `brand`, `mrp`, `price`, `stock`, `available`, `unit`, `weight_g`, `category`, `image_url`, `updated_at`. Money unit (rupees or cents) is chosen in the mapping. | M |
| FR-SYNC-06 | Preview first 20 mapped rows before saving. | M |
| FR-SYNC-07 | Incremental sync when an `updated_at` column is mapped; otherwise full read with row hash compare. | M |
| FR-SYNC-08 | Runs on the signed trigger URL (instant), on "Sync now", and on a safety schedule (`interval_min`, default 60, min 10, open hours only). Runs never overlap. | M |
| FR-SYNC-09 | Rows match master products by barcode, then by saved link. No match → `sync_issues` (`UNMATCHED`). Optional "auto-create master product" per connector. | M |
| FR-SYNC-10 | Safety holds: new price > MRP, or price drop > 50%, or price 0 → `sync_issues` (`PRICE_HOLD`), old price stays. | M |
| FR-SYNC-11 | Rows missing from source (full sync) → product set unavailable, never deleted. | M |
| FR-SYNC-12 | Every run writes `connector_runs` (counts, duration, error). 3 failed runs in a row → admin alert. | M |
| FR-SYNC-13 | Limits: query timeout 20 s, max 20,000 rows per run, host must not be a private / local address. | M |

## F6 — Home and search

| ID | Requirement | P |
|---|---|---|
| FR-HOME-01 | One call `GET /v1/home` returns: shops near the location, lowest-price items, promotions. | M |
| FR-HOME-02 | 2-way search bar: shop (optional) + product text. Product-only search lists items across shops, cheapest first. | M |
| FR-HOME-03 | Search uses `pg_trgm` on name + keywords (handles spelling mistakes). | M |
| FR-HOME-04 | Public catalog responses are cacheable for 60 s. | M |

## F7 — Cart

| ID | Requirement | P |
|---|---|---|
| FR-CART-01 | Each shop is one tenant in the cart service. The browser uses that shop's public key. | M |
| FR-CART-02 | Only one active cart in the browser at a time (one shop). Adding from another shop asks to clear. | M |
| FR-CART-03 | Cart works before sign-in. | M |
| FR-CART-04 | Cart prices are display only. The bill always re-prices from our database. | M |

## F8 — Bill and shop review

| ID | Requirement | P |
|---|---|---|
| FR-BILL-01 | `POST /v1/bills/quote` returns a full bill preview (no write). Called only when cart, address or slot changes. | M |
| FR-BILL-02 | `POST /v1/bills` (with `Idempotency-Key`) re-checks everything, books the slot atomically, writes the order `SUBMITTED`, deletes the cart, publishes `order.submitted`. | M |
| FR-BILL-03 | Checks on submit: signed in, phone verified, fewer than 3 open unpaid orders, shop can take orders, address in range, min order reached, slot open and not full, lines available, quantity ≤ max per order, ≤ 60 lines. | M |
| FR-BILL-04 | Each line has a substitution choice: `REMOVE`, `SIMILAR`, `CALL_ME`. | M |
| FR-BILL-05 | Shop review actions: `APPROVE`, `REVISE` (change qty, remove, substitute, change slot), `REJECT` (reason). | M |
| FR-BILL-06 | Prices and fees are **frozen** at approval (or at customer acceptance of a revision). | M |
| FR-BILL-07 | A revision shows the customer old vs new. The customer must accept before paying. | M |
| FR-BILL-08 | Timers: review (default 15 min), revision answer (15 min), payment (30 min). On timeout → `EXPIRED`, slot released. | M |
| FR-BILL-09 | Optimistic locking: every order update checks `version`. Conflict → `409 STALE_ORDER`. | M |

## F9 — Slots and delivery fee

| ID | Requirement | P |
|---|---|---|
| FR-SLOT-01 | Slot = shop, start, end, capacity, booked, cutoff. Managed by admin (one by one or "copy last week"). | M |
| FR-SLOT-02 | Booking is atomic: `UPDATE … SET booked = booked + 1 WHERE booked < capacity AND now() < cutoff_at`. | M |
| FR-SLOT-03 | Released on `REJECTED`, `EXPIRED`, `CANCELLED`. Changed slot on revision moves the booking. | M |
| FR-FEE-01 | Fee rule fields: base, per km after free km, per kg after free kg, free above order value, min, max, max km. | M |
| FR-FEE-02 | Shop rule overrides platform default. Fee saved on the order. | M |
| FR-FEE-03 | Admin preview calculator for a rule. | S |

## F10 — Coupons

| ID | Requirement | P |
|---|---|---|
| FR-CPN-01 | Coupon: code, type (`PERCENT` / `FIXED`), value, max discount, min order, start / end, total limit, per-user limit, shop scope (one shop or all), funded by (`PLATFORM` / `SHOP`). | M |
| FR-CPN-02 | Applied only in `AWAITING_PAYMENT`. One per order. Validated on the server. | M |
| FR-CPN-03 | Applying reserves one use atomically. Released if the order expires or is cancelled before payment. | M |
| FR-CPN-04 | A platform-funded discount is capped so the order's platform income (margin + service fee) never goes below zero, unless the admin ticks "allow loss". | M |
| FR-CPN-05 | Codes are case-insensitive. Wrong-code attempts are rate limited (10 / hour / user). | M |

## F11 — Payment

| ID | Requirement | P |
|---|---|---|
| FR-PAY-01 | Providers: `PAYHERE`, `MANUAL`. Chosen per shop by admin. | M |
| FR-PAY-02 | PayHere account owner: `PLATFORM` (default) or `SHOP`. Account mode `SANDBOX` / `LIVE`. Secrets in SSM. | M |
| FR-PAY-03 | `POST /v1/orders/{id}/payment` returns the signed PayHere checkout fields. Amount comes from the frozen order. | M |
| FR-PAY-04 | `PAID` only after the notify callback passes: signature, merchant id, order id, amount, currency, status code, order state. | M |
| FR-PAY-05 | Callback is idempotent (unique provider reference). | M |
| FR-PAY-06 | Successful payment that arrives after the order left `AWAITING_PAYMENT` (expired / cancelled) → payment recorded, a refund request (`LATE_PAYMENT`) is created automatically for the admin. | M |
| FR-PAY-07 | Manual mode: customer sees bank details + order code. Admin marks paid with reference. | M |

## F12 — Fulfilment and delivery

| ID | Requirement | P |
|---|---|---|
| FR-DLV-01 | Shop moves `PAID → PICKING → PACKED → OUT_FOR_DELIVERY`. A driver must be assigned before `OUT_FOR_DELIVERY`. | M |
| FR-DLV-02 | Driver source per shop: `PLATFORM`, `SHOP`, `BOTH`. Platform drivers are assigned by admin. | M |
| FR-DLV-03 | At `OUT_FOR_DELIVERY` a 4-digit delivery code is created, stored encrypted, shown to the customer and sent by SMS. Never sent to the driver. | M |
| FR-DLV-04 | Driver enters the code → `DELIVERED`. 5 wrong tries → locked, admin unlocks. | M |
| FR-DLV-05 | `DELIVERY_FAILED` with reason. Shop / admin can retry (`OUT_FOR_DELIVERY` again, new code). | M |
| FR-DLV-06 | Pick list sorted by category, shows unit, qty, substitution choice. | M |
| FR-DLV-07 | `COURIER` delivery type exists as a stub only. | M |

## F13 — Cancels and refunds

| ID | Requirement | P |
|---|---|---|
| FR-RFD-01 | Customer can cancel only before payment. | M |
| FR-RFD-02 | Shop can request full cancel (before `PACKED`) or partial refund (a missing line, while `PICKING`). | M |
| FR-RFD-03 | Admin approves / rejects. Approve full cancel → order `CANCELLED_REFUND`. | M |
| FR-RFD-04 | Refund records: amount, reason, method (`MANUAL` now), status, reference. Ledger reversal rows on completion. | M |
| FR-RFD-05 | Customer "Report a problem" on paid orders goes to the admin queue. | M |

## F14 — Notifications

| ID | Requirement | P |
|---|---|---|
| FR-NTF-01 | Channels per shop: any of `SMS`, `WHATSAPP`, `WA_LINK` (none selected = off). OTPs always by SMS. | M |
| FR-NTF-02 | Events that notify: bill submitted (shop), approved / revised / rejected (customer), paid (shop), out for delivery + code (customer), delivered (customer), expired (both). | M |
| FR-NTF-03 | Every send is logged with masked phone, template, status, cost units. | M |
| FR-NTF-04 | Caps: per phone 5 OTP / day, platform daily SMS cap from env, alert at 80%. | M |
| FR-NTF-05 | Templates use `PLATFORM_NAME` and shop name. No links except to `PLATFORM_DOMAIN`. | M |

## F15 — Dashboards and analytics

| ID | Requirement | P |
|---|---|---|
| FR-DASH-01 | Owner: sales, orders, average bill, top 10 products, cancel rate, margin deducted, payout due — by day / week / month. | M |
| FR-DASH-02 | Staff: today's queue, average review time, average pick time. | M |
| FR-DASH-03 | Driver: deliveries today / week. | M |
| FR-DASH-04 | Admin: all shops, GMV, platform margin, service fees, coupons cost, SMS used, failed payments, sync health. | M |
| FR-DASH-05 | Dashboards read `shop_daily_stats` (updated by the worker), never scan all orders. | M |

## F16 — Ledger and payouts

| ID | Requirement | P |
|---|---|---|
| FR-FIN-01 | On `PAID`, write ledger rows: sale, platform margin, service fee, delivery fee, discount (by funder). Append only. | M |
| FR-FIN-02 | Refunds write reversing rows. Never edit or delete ledger rows. | M |
| FR-FIN-03 | Payout = sum of the shop's due amount for a period. Admin marks paid with bank reference. | M |
| FR-FIN-04 | PayHere fee recorded when known (admin enters from statement in phase 1). | S |

## F17 — Buy again

| ID | Requirement | P |
|---|---|---|
| FR-BUY-01 | On `DELIVERED`, upsert `customer_products` (user, shop, product, times, last date). No price stored. | M |
| FR-BUY-02 | `GET /v1/shops/{slug}/buy-again` joins with current shop products → current price + available. | M |

## F18 — Admin, team and audit

| ID | Requirement | P |
|---|---|---|
| FR-ADM-01 | Audit log for: price, settings, payment config, connector, coupon, role, refund, order overrides. Who, what, when, before / after. | M |
| FR-ADM-02 | Admin can search users, orders, shops. | M |
| FR-ADM-03 | Admin can unlock a delivery code, retry delivery, force-expire an order (with reason). | M |
| FR-ADM-04 | Shop owner manages own staff and shop drivers (invite, role, remove). | M |

---

## Non-functional requirements

| ID | Area | Requirement |
|---|---|---|
| NFR-PERF-01 | Speed | API p95 < 400 ms warm, < 1.5 s cold. Home page usable in < 3 s on 4G mid-range Android. |
| NFR-PERF-02 | Bundle | Customer first-load JS < 180 KB gzip. Console loaded only on `/console`. |
| NFR-PERF-03 | Calls | Home = 2 API calls max (`bootstrap`, `home`). Shop page = 2 (`shop`, `products`). Bill page = 1 `quote` + 1 `submit`. |
| NFR-SEC-01 | Security | All items in `11-security-audit.md` pass before go-live. |
| NFR-SEC-02 | Isolation | Cross-shop and cross-customer access tests exist for every scoped endpoint. |
| NFR-COST-01 | Cost | Infra < USD 10 / month at phase 1 traffic (excluding SMS and PayHere fees). AWS budget alert at USD 5. |
| NFR-COST-02 | DB | No scheduled job may touch Neon more often than every 10 minutes, and only in open hours. |
| NFR-AVL-01 | Availability | If the cart service is down, browsing still works; "Add" shows a retry message. |
| NFR-DATA-01 | Backups | Neon point-in-time restore on. Restore tested once before go-live. |
| NFR-PRIV-01 | Privacy | No full phone numbers, addresses or tokens in logs. Phones masked `07X•••1234`. |
| NFR-A11Y-01 | Access | WCAG 2.1 AA colors, 44 px tap targets, keyboard usable console. |
| NFR-I18N-01 | Language | All UI text in `web/src/i18n/en.json`. Sinhala and Tamil files can be added without code change. |
| NFR-TIME-01 | Time | Store UTC. Show `Asia/Colombo`. Slots and open hours are evaluated in `Asia/Colombo`. |
| NFR-MAINT-01 | Code | No file over 300 lines. No duplicated helper (see `07` and `08` reuse tables). |
