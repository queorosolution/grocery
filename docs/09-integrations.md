# 09 — Integrations

Every outside system sits behind **one Go package** with a small interface, so tests can replace it and the rest of the code never sees HTTP details.

| System | Package | Secret (SSM name) |
|---|---|---|
| Cart service | `internal/cartclient` | `/<app>/<env>/cart/platform-key` |
| PayHere | `internal/payment/payhere` | per account: `/<app>/<env>/payhere/<accountId>` |
| SMS gateway | `internal/notify/sms` | `/<app>/<env>/sms/api-key` |
| WhatsApp Cloud API | `internal/notify/whatsapp` | `/<app>/<env>/whatsapp/token` |
| Google sign-in | `internal/auth/google` | none (client id is public) |
| Google Maps | browser only | browser key (public, restricted) |
| Shop source DB | `internal/connector/source` | per connector: `/<app>/<env>/connectors/<id>` |
| R2 | `internal/upload` | `/<app>/<env>/r2/keys` |
| Cloudflare Pages deploy hook | `internal/site` | `/<app>/<env>/pages/deploy-hook` |

---

## 1. Cart service (your existing service)

### 1.1 Setup per shop (admin → "Enable cart")

`POST /admin/shops/{id}/cart-tenant` makes the core API call the cart service **Admin API** with the **platform key**:

| Field sent | Value |
|---|---|
| `externalId` | our `shops.id` (safe to repeat: `409` returns the existing tenant) |
| `name` | shop name |
| `currency` | `LKR` |
| `allowedOrigins` | `ALLOWED_ORIGINS` (e.g. `https://www.nearshop.lk`, dev origin) |
| `plan` | `pro` (a constant in `cartclient`). The plan must allow more items than the shop has products; if it does not, the upload fails with `QUOTA_EXCEEDED` and the DLQ alarm fires (open question Q4) |

Saved in `shop_settings`: `cart_tenant_id`, `cart_public_key`. The secret key is not stored — the core uses the platform key + `X-Tenant-Id`.

- `CART_API_BASE` ends in `/v1`; the admin API is derived from it (`…/admin/v1`). The platform key is read from SSM `cart/platform-key` on every call and never logged. Every call has a 3 s timeout; a network error, a `5xx`, `408` or `429` is retried twice (150 ms, 400 ms). A `4xx` and `QUOTA_EXCEEDED` are not retried. Failures map to `503 CART_UNAVAILABLE`.
- The cart service is called **between** two database transactions, never inside one. If the tenant was created but the save failed, a repeat finds it again (`409` carries `details.tenantId`; the public key is read from its key list, or created).
- When the tenant is saved, the same transaction queues a **full price upload** (`catalog.changed` events for every product, 200 ids each).

### 1.2 Who calls what

| Caller | Key | Calls |
|---|---|---|
| Browser | shop **public** key | create cart, add / change / remove lines (cart service line endpoints as in its API doc) |
| Core API | **platform** key + `X-Tenant-Id` | `GET /v1/carts/{id}` (quote, submit), `DELETE /v1/carts/{id}` (after submit), `POST /v1/items/batch` (price sync) |

- **Price list item id = our `shop_products.id`.** `unitPrice` = `online_price_cents`. `active` = available. `maxQuantity` = `max_per_order`.
- On submit the core checks the cart belongs to the shop's tenant (else `CART_WRONG_SHOP`) and is `open`.
- Cart service orders, checkout and webhooks are **not used** by this platform (the core owns orders).

### 1.3 Price sync to the cart

- `catalog.changed` events carry up to 200 `shopProductIds`.
- Worker (`catalog.CartSync`, consumer name `cart.prices`) re-reads them and sends `POST /v1/items/batch` in chunks of **25** (the cart service limit). A shop without a cart tenant is skipped. An item the cart service would refuse (name over 120, price over 999,999,999, bad `maxQuantity`) is skipped and counted in the log; the others are sent. A failed upload fails the event, so the queue retries it (uploading an item again is harmless).
- The catalog API caps a shop price at Rs 5,000,000 (500,000,000 cents) so that, with the largest markup (20%), the customer price stays under the cart service's limit.
- After every connector run: one event per 200 changed ids.
- Nightly (`stats-rollup`): no full re-sync by default. Admin can press "Re-sync cart prices" for a shop (sends all items in batches).
- Remember: the cart price is display only. **The bill always uses Neon prices.**

---

## 2. PayHere

Based on PayHere's Checkout API documentation. **Re-check the official docs before go-live.**

### 2.1 Before you start

- Create a **sandbox** merchant account for development; the live account comes later.
- PayHere approves **each domain / app** and issues a merchant secret for it; approval can take up to 24 hours. Add both the dev and prod web domains.
- `notify_url` must be public HTTPS (`https://api.<domain>/v1/webhooks/payhere`). It cannot be localhost.

### 2.2 Checkout (server builds, browser posts)

| Item | Value |
|---|---|
| Action URL | Sandbox `https://sandbox.payhere.lk/pay/checkout` · Live `https://www.payhere.lk/pay/checkout` |
| Encoding | `application/x-www-form-urlencoded` form POST from the browser |
| `order_id` | our **payment id** (`payments.id`, one per attempt) |
| `amount` | frozen `total_cents` formatted with 2 decimals, no thousands separator: `158500` → `1585.00` |
| `currency` | `LKR` |
| `hash` | `UPPER( MD5( merchant_id + order_id + amount + currency + UPPER( MD5(merchant_secret) ) ) )` |
| `return_url` | `https://www.<domain>/order/<orderId>?paid=1` (PayHere sends **no** status here) |
| `cancel_url` | `https://www.<domain>/order/<orderId>` |
| `notify_url` | `https://api.<domain>/v1/webhooks/payhere` |

- The hash is made **only on the server**. The merchant secret never reaches the browser.
- A new `payments` row (`INITIATED`) is created for each attempt.

### 2.3 Notify handler — exact checks, in order

PayHere posts: `merchant_id`, `order_id`, `payment_id`, `payhere_amount`, `payhere_currency`, `status_code`, `md5sig`, `custom_1`, `custom_2`, `method`, `status_message` (and card fields for card payments).

1. Parse form (max 16 KB). Missing field → `400`.
2. Load `payments` row by `order_id` (our payment id). Not found → log, `200` (ignore).
3. Load its payment account; read the merchant secret from SSM.
4. Compute `UPPER( MD5( merchant_id + order_id + payhere_amount + payhere_currency + status_code + UPPER( MD5(merchant_secret) ) ) )`.
5. Compare with `md5sig` using constant-time compare. Mismatch → `400`, log `payhere.bad_signature` (no secrets).
6. `merchant_id` must equal the account's merchant id.
7. `payhere_currency` must be `LKR`; `payhere_amount` must equal the payment amount (`1585.00` vs `158500`).
8. Upsert by unique `(provider, provider_ref = payment_id)` → duplicates do nothing.
9. By `status_code`:

| Code | Meaning | Action |
|---|---|---|
| `2` | success | payment `SUCCESS`; if order is `AWAITING_PAYMENT` → T10; else create refund `LATE_PAYMENT` |
| `0` | pending | keep `INITIATED` |
| `-1` | cancelled | payment `CANCELLED` |
| `-2` | failed | payment `FAILED` |
| `-3` | charged back | payment `CHARGEBACK`, alert admin |

10. Save a **sanitised** copy in `payments.raw`: drop `card_holder_name`, `card_no`, `card_expiry`, `md5sig`.
11. Answer `200` fast.

### 2.4 Refunds

Phase 1: refunds are done by the admin outside the system (PayHere merchant portal or bank transfer), then recorded with `POST /admin/refunds/{id}/paid-out { reference }`. Automatic refunds are phase 3 (check if PayHere offers a refund API for your account type).

---

## 3. SMS and WhatsApp

### 3.1 SMS (phase 1)

- Generic HTTP adapter. Config: `SMS_API_URL`, `SMS_SENDER_ID`, key in SSM. Choose a Sri Lankan provider (open question Q2) and write one small adapter file for its request format.
- Sender ID is usually registered with the provider under your brand → do it **after** the platform name is final.
- `dev` uses the log channel (no real SMS).

**Templates** (`internal/notify/templates.go`; `{app}` = `PLATFORM_NAME`):

| Key | To | Text |
|---|---|---|
| `otp.phone` | customer | `{app}: your code is {code}. Valid 10 min. Do not share it.` |
| `shop.new_bill` | shop phone | `{app}: new bill {orderCode} ({items} items). Please review.` |
| `order.approved` | customer | `{app}: {shop} confirmed order {orderCode}. Pay here: https://www.{domain}/order/{id}` |
| `order.revised` | customer | `{app}: {shop} updated order {orderCode}. Please check: https://www.{domain}/order/{id}` |
| `order.rejected` | customer | `{app}: sorry, {shop} could not accept order {orderCode}.` |
| `order.paid` | shop phone | `{app}: order {orderCode} is paid. Slot {slot}.` |
| `order.out` | customer | `{app}: order {orderCode} is on the way. Give code {code} to the driver.` |
| `order.delivered` | customer | `{app}: order {orderCode} delivered. Thank you!` |
| `order.expired` | customer | `{app}: order {orderCode} expired.` |

Rules: only `https://www.<PLATFORM_DOMAIN>` links; variables have control characters removed and are trimmed; max 160 characters (check in a unit test).

### 3.2 WhatsApp

- `WA_LINK` (free, phase 1): console shows "Send on WhatsApp" which opens `https://wa.me/<phone>?text=<url-encoded template>`. Staff taps send.
- `WHATSAPP` Cloud API (phase 2): needs a Meta business account, a verified number, and **pre-approved templates**; billed per conversation. Enabled with `FEATURE_WHATSAPP=true`.

---

## 4. Google

### 4.1 Sign-in

- Google Identity Services button in the sign-in modal. Script loaded only when the modal opens.
- OAuth client: authorized JavaScript origins = the web origins (prod + dev).
- Server checks with `go-oidc`: issuer `https://accounts.google.com`, audience `GOOGLE_CLIENT_ID`, expiry, `email_verified = true`. Uses `sub` as the stable id.

### 4.2 Maps

- **Browser key only.** Restrictions: HTTP referrers = web origins; APIs = Maps JavaScript API + Places API. Set a daily quota.
- Places Autocomplete with **session tokens** (one billing session per address search).
- No server-side Google calls: distance is haversine (D13).

---

## 5. Shop source database (Krishna Super, Neon)

### 5.1 What Krishna Super's database owner runs once

```sql
-- In Krishna Super's Neon project
CREATE ROLE gp_reader LOGIN PASSWORD '<long random password>';
ALTER ROLE gp_reader SET default_transaction_read_only = on;

-- Expose only what the platform needs (adjust column names to the real table)
CREATE VIEW public.gp_products AS
SELECT id         AS external_id,
       barcode,
       name,
       brand,
       mrp,
       price,
       stock,
       is_active  AS available,
       updated_at
FROM public.products;

GRANT USAGE ON SCHEMA public TO gp_reader;
GRANT SELECT ON public.gp_products TO gp_reader;
```

Then in our admin: paste `postgresql://gp_reader:<password>@<host>/<db>?sslmode=require` → Test → pick `public.gp_products` → map → preview → activate.

### 5.2 Instant updates (so changes "show directly")

Krishna Super's own admin panel calls our trigger URL **after** saving a price or stock change:

```js
// Node.js 18+ example for the shop's admin panel (server side only)
import crypto from "node:crypto";

export async function notifyPlatform(externalIds) {
  const body = JSON.stringify({ externalIds });            // up to 100 ids
  const ts = Math.floor(Date.now() / 1000).toString();
  const sig = crypto.createHmac("sha256", process.env.GP_TRIGGER_SECRET)
                    .update(`${ts}.${body}`).digest("hex");
  await fetch(`https://api.nearshop.lk/v1/connectors/${process.env.GP_CONNECTOR_ID}/trigger`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-GP-Timestamp": ts, "X-GP-Signature": `v1=${sig}` },
    body,
  });
}
```

- The trigger secret is shown **once** when the admin rotates it. Keep it in the shop system's server env, never in a browser.
- Our handler: verify timestamp (±5 min) and HMAC (constant time) → invoke `sync` asynchronously with those ids → `202`.

### 5.3 Neon free-plan care (both projects)

- Every sync run wakes **both** databases. A run every 10 minutes all day would use most of a free compute allowance.
- So: **instant trigger** for real changes + a **safety schedule every 60 minutes** during open hours (`interval_min` default 60).
- A run with no changes does one small query and stops.

### 5.4 Mapping fields

| Our field | Required | Notes |
|### 5.5 How the server connects (P16)

- **TLS is checked.** `sslmode` may be `require`, `verify-ca` or `verify-full`; the server verifies the certificate chain and the host name in every case.
- **SSRF guard.** The host is resolved once, every answer must be a public address, and the connection goes to that address (no second lookup). Private, loopback, link-local, multicast and cloud-metadata ranges are refused. Only port 5432 is allowed.
- **Read-only per transaction.** Every read runs in `BEGIN READ ONLY` with `SET LOCAL statement_timeout`. This is done per transaction, not as a start-up parameter, so it also works through Neon's pooler. The server does not trust the login to be read-only; `test` warns when it is not.
- **Allowed string parts.** `user`, `password`, host, port, database, `sslmode`, `channel_binding`, `connect_timeout`, `application_name`. Everything else is refused.
- **Identifiers.** Table and column names are checked against `information_schema` first and quoted with `pgx.Identifier`. They never come straight from the request into SQL.
- **Mapping keys** are the `Our field` names in 5.4. `updatedAt` also feeds `changeColumn` when none is given.
- **Limits.** 30 s per call; preview reads 20 rows ordered by `externalId`.

---|---|---|
| `externalId` | yes | stable id in the source |
| `name` | yes | |
| `price` | yes | shop price; unit set by `moneyUnit` (`RUPEES` → × 100) |
| `barcode` | no | best key for matching master products |
| `mrp` | no | if missing, no MRP cap |
| `stock` | no | `≤ 0` → unavailable |
| `available` | no | boolean |
| `brand`, `unit`, `weightG`, `category`, `imageUrl` | no | used when auto-creating master products |
| `updatedAt` | no | enables incremental sync |

---

## 6. Cloudflare R2 (images)

- Public bucket behind custom domain `img.<PLATFORM_DOMAIN>` (Cloudflare caches it).
- Bucket CORS: allow `PUT` from the web origins, header `Content-Type`.
- Presign with the S3 API (`https://<accountId>.r2.cloudflarestorage.com`), `PutObject` with fixed `ContentType` and `ContentLength`, 5 min expiry.
- Object keys are server-made UUIDs. Old images are not deleted automatically (cheap); an admin clean-up is phase 3.

## 7. AWS services used directly

| Service | Used for | Notes |
|---|---|---|
| SQS | `events` queue + DLQ | Standard queue, long polling via Lambda trigger, `ReportBatchItemFailures` |
| EventBridge Scheduler | Order timers, sync schedules, sweep, nightly | One schedule group per env. `ActionAfterCompletion: DELETE` for one-time timers |
| SSM Parameter Store | All secrets | `SecureString`, standard tier. Lambda role can read only `/<app>/<env>/*` |
| Lambda async invoke | `sync` from trigger / "Sync now" | `InvocationType: Event` |
| CloudWatch | Logs (14 days), alarms | Alarms: 5xx > 1%, DLQ not empty, sync failing, payment bad signature |

## 8. Cloudflare Pages deploy hook (site rebuild)

- Create it in Cloudflare Pages → project → Settings → Builds → **Deploy hooks** (branch `main` for prod, the dev branch for dev).
- The hook URL is a **secret** (anyone with it can start builds): store it in SSM `pages/deploy-hook`, never in the web app or repo.
- Called only by the worker for `site.rebuild` events (coalesced, see `07-backend-design.md` §10).
- The build runs `scripts/shop-slugs.mjs`, which calls `GET /v1/shops?fields=slug` to know which shop pages to pre-render.
- Price, stock and product changes **never** trigger a rebuild — the shop page loads them live from the API.
