# 06 — API reference (v1)

Base URL: `https://api.<PLATFORM_DOMAIN>/v1`

## 1. Conventions

| Topic | Rule |
|---|---|
| Format | JSON, `camelCase` fields. Money fields end in `Cents` (integer). Times are ISO-8601 UTC. |
| Auth | Cookies `gp_at` (access, 15 min) and `gp_rt` (refresh, 30 days, path `/v1/auth`). Browser sends them with `credentials: "include"`. |
| CSRF | Every non-GET request must send header `X-Requested-With: gp-web` **and** an `Origin` in `ALLOWED_ORIGINS`. Otherwise `403`. Webhooks are exempt (they use signatures). |
| Body size | Max 64 KB (`413 PAYLOAD_TOO_LARGE`). Webhook max 16 KB. |
| Unknown fields | Rejected (`400 VALIDATION_ERROR`). Decoder uses `DisallowUnknownFields`. |
| Idempotency | `POST /bills`, `POST /orders/{id}/payment`, review and status changes accept `Idempotency-Key` (16–64 chars). Same key + same body → same response. Same key + different body → `409 IDEMPOTENCY_MISMATCH`. |
| Concurrency | Order-changing requests send the order `version`. Wrong version → `409 STALE_ORDER`. |
| Pagination | `?limit=` (max 50, default 20) and `?cursor=` (opaque). Response: `{ "data": [...], "nextCursor": "…" | null }`. |
| Caching | Public `GET`s send `Cache-Control: public, max-age=60`. Signed-in `GET`s send `Cache-Control: private, no-store`. Console list sends `ETag`. |
| Request id | Every response has `X-Request-Id`. Errors include it. |

**Error shape** (codes and default messages are in `contracts/enums.json` → `error_codes`):

```json
{
  "error": {
    "code": "SLOT_FULL",
    "message": "That time slot is full. Please pick another.",
    "fields": { "slotId": "full" },
    "requestId": "req_01JC…"
  }
}
```

**Auth column legend:** `public` · `user` (any signed-in user) · `perm:x` (permission + scope check) · `admin` (super admin with TOTP) · `sig` (signature only).

## 2. Rate limits

| What | Limit | Where enforced |
|---|---|---|
| All routes per IP | 300 / 5 min | Cloudflare rate-limit rule |
| HTTP API stage | 50 req/s, burst 100 | API Gateway throttling |
| `POST /auth/google` | 10 / 10 min per IP | app (`rate_counters`) |
| `POST /me/phone/send` | 3 / hour per user, 5 / day per phone | app |
| `POST /me/phone/verify` | 5 tries per code | app |
| `POST /orders/{id}/coupon` | 10 / hour per user | app |
| `POST /driver/deliveries/{id}/complete` | 5 tries per code | app |
| `POST /connectors/{id}/trigger` | 30 / hour per connector | app |
| `POST /bills` | 10 / hour per user | app |

---

## 3. Public

### `GET /health` — public
`200 {"ok": true}`. No database call.

### `GET /bootstrap` — public (user part if signed in)
One call per app load.

```json
{
  "platform": { "name": "NearShop", "domain": "nearshop.lk", "supportPhone": "+9477xxxxxxx", "singleShopSlug": null },
  "features": { "whatsapp": false, "manualPayment": true },
  "me": {
    "id": "…", "name": "Nimal", "email": "nimal@gmail.com",
    "phoneMasked": "077•••4567", "phoneVerified": true, "needsOnboarding": false,
    "roles": [ { "role": "SHOP_OWNER", "shopId": "…", "shopSlug": "krishna-super", "shopName": "Krishna Super" } ],
    "adminTotp": "NOT_REQUIRED"
  }
}
```

- `me` is `null` when signed out. `adminTotp`: `NOT_REQUIRED` | `SETUP_NEEDED` | `CODE_NEEDED` | `OK`.

### `GET /home?lat=&lng=` — public, cacheable
`lat`/`lng` rounded by the client to 2 decimals (keeps the cache useful and hides the exact location).

```json
{
  "shops": [ { "id": "…", "slug": "krishna-super", "name": "Krishna Super", "logoUrl": "…", "themeColor": "#0F5132", "openNow": true, "nextSlot": "2026-11-02T08:30:00Z", "distanceKm": 1.8 } ],
  "deals": [ { "shopProductId": "…", "shopSlug": "krishna-super", "shopName": "Krishna Super", "name": "Samba Rice 5kg", "imageUrl": "…", "onlinePriceCents": 143500, "mrpCents": 150000, "saveCents": 6500 } ],
  "promos": [ { "shopSlug": "krishna-super", "title": "Weekend offers", "imageUrl": "…" } ]
}
```

### `GET /search?q=&shop=&cursor=` — public, cacheable
- `q` 2–60 characters. `shop` optional slug.
- Without `shop`: products across shops that deliver near `lat,lng` (optional params), cheapest first, plus matching shops.

```json
{ "shops": [ … ], "products": { "data": [ { "shopProductId": "…", "shopSlug": "…", "name": "…", "onlinePriceCents": 0, "available": true } ], "nextCursor": null } }
```

### `GET /shops?fields=slug` — public, cacheable
Active shops: `{ "data": [ { "slug": "krishna-super", "name": "Krishna Super", "logoUrl": "…", "updatedAt": "…" } ] }`. Used by the web build to pre-render shop pages.

### `GET /shops/{slug}` — public, cacheable

```json
{
  "id": "…", "slug": "krishna-super", "name": "Krishna Super", "logoUrl": "…", "themeColor": "#0F5132",
  "openNow": true, "acceptingOrders": true, "hoursToday": { "opens": "08:00", "closes": "21:00" },
  "deliveryType": "SLOT", "minOrderCents": 100000,
  "cart": { "tenantId": "ten_…", "publicKey": "pk_live_…" },
  "categories": [ { "id": "…", "slug": "rice", "name": "Rice", "count": 42 } ]
}
```

### `GET /shops/{slug}/products?category=&q=&cursor=&limit=` — public, cacheable

```json
{ "data": [ { "shopProductId": "…", "productId": "…", "name": "Samba Rice", "brand": "Araliya", "sizeLabel": "5kg", "unit": "PCS",
  "imageUrl": "…", "onlinePriceCents": 143500, "mrpCents": 150000, "available": true, "maxPerOrder": 20 } ], "nextCursor": "…" }
```

### `GET /shops/{slug}/slots?days=3` — public
Open slots for the next `days` (1–7, default 3; today counts as day 1, Colombo time) with `remaining = capacity − booked` (never below 0) and `cutoffAt`.
- Only slots that are `OPEN` and before their cutoff are listed. A full slot is listed with `remaining: 0`, so the page can show "full".
- Not cached (the numbers change with every order). Unknown or not-active shop → `404`.
- `{ "data": [ { "id": "…", "startsAt": "…", "endsAt": "…", "cutoffAt": "…", "remaining": 3 } ] }`

### `GET /categories` — public, cacheable
Category tree.

---

## 4. Auth

### `POST /auth/google` — public
Body: `{ "idToken": "…", "inviteToken": "…optional…" }`
- Verifies the Google token. Creates the user on first sign-in. Accepts the invite if given (email must match).
- Sets cookies. Returns the same `me` object as `/bootstrap`.
- Errors: `UNAUTHENTICATED` (bad token), `FORBIDDEN` (blocked user, invite email mismatch), `RATE_LIMITED`.
- `inviteToken` (optional): the invite must be usable (unknown, used, revoked or expired → `404 NOT_FOUND`) and its e-mail must equal the Google e-mail (`403`). A refused accept signs nobody in and creates nothing.
- If the e-mail is in `SUPER_ADMIN_EMAILS`, a `PLATFORM / SUPER_ADMIN` membership is created on sign-in (still needs TOTP for admin routes).
- The limit is 10 attempts per 10 minutes per IP (before the token is checked) and per Google account (after).

### `POST /auth/totp/setup` — user with SUPER_ADMIN membership, `adminTotp = SETUP_NEEDED`
Returns `{ "otpauthUri": "otpauth://totp/…", "secretMasked": "ABCD••••••••" }` once. Show as QR. Can be repeated until the first code is accepted (the pending secret is replaced); after that `403`. The secret is stored AES-256-GCM encrypted.

### `POST /auth/totp/verify` — same user
Body `{ "code": "123456" }`. Marks the session `totp_ok`, sets a new `gp_at` cookie (the refresh cookie is unchanged) and returns `me`. First verify also enables TOTP. A code works once (`last_used_step`); one step of clock drift is allowed. Errors: `TOTP_INVALID`, `RATE_LIMITED` (5 tries / 15 min per user, counted whether right or wrong), `FORBIDDEN` (not a super admin, or no setup yet).

### `POST /auth/refresh` — refresh cookie
Rotates the refresh token and sets new cookies, `204`. Reuse of an old token → whole family revoked → `401` and the `auth.refresh_reuse` alarm event. A refused refresh also clears both cookies. A blocked user or an expired token → `401`.

### `POST /auth/logout` — user
Body `{ "all": false }` (optional). Ends this sign-in, or every sign-in of the user when `all` is true. Clears cookies, `204`.

---

## 5. Me (customer)

| Method | Path | Auth | Body / query | Result |
|---|---|---|---|---|
| `GET` | `/me` | user | — | `me` object |
| `PATCH` | `/me` | user | `{ "name": "…" }` (1–80 characters; e-mail cannot be changed) | `me` |
| `POST` | `/me/phone/send` | user | `{ "phone": "0771234567", "turnstileToken": "…" }` (phone normalised to `+94…`; mobile numbers only, `07x`; Turnstile verified server-side) | `204` · `VALIDATION_ERROR` (`fields.phone`), `BOT_CHECK_FAILED`, `RATE_LIMITED`. Limits: 3 sends / hour / user and 5 / day / phone. A new code replaces the old one. The code has 6 digits and works for 10 minutes. |
| `POST` | `/me/phone/verify` | user | `{ "code": "123456" }` (4–6 digits) | `me` (`phoneVerified: true`) · `OTP_INVALID` (wrong, expired or none sent) · `OTP_LOCKED` (from the 6th try on that code; send a new one) |
| `GET` | `/me/addresses` | user | — | `{ "data": [address] }` |
| `POST` | `/me/addresses` | user | `{ label, line1, landmark, lat, lng, isDefault }` | `201 { address, deliveringShops: [ {slug, name, distanceKm} ] }` |
| `PATCH` | `/me/addresses/{id}` | user | same fields, all optional, at least one; `landmark: ""` clears it | `{ address, deliveringShops }` (same shape as create, so the page can re-check after the pin moves) |
| `DELETE` | `/me/addresses/{id}` | user | — | `204`. If it was the default, the newest remaining address becomes the default. |

Address rules: `label` 1–30, `line1` 3–160, `landmark` ≤ 120 characters; the pin must be inside Sri Lanka (`lat` 5.8–10.0, `lng` 79.4–82.0); at most 5 per user (`400`, `fields.addresses = "max_5"`); the first address is the default; `isDefault: true` moves the default; the default cannot be switched off, only moved (`fields.isDefault`). Another user's address is `404`.

`deliveringShops` lists the active shops whose delivery distance covers the pin (straight line × road factor ≤ `maxKm` of the shop's fee rule, else the platform default), nearest first. `distanceKm` is the straight line with two decimals. A shop with no fee rule at all is not listed.
| `DELETE` | `/me` | user | `{ "confirm": "DELETE" }` | `204`, account anonymised (not built yet: it needs orders, so it comes with the order prompts) |
| `GET` | `/shops/{slug}/buy-again` | user | — | `{ "data": [ product + available + current price ] }` |

---

## 6. Bills and orders (customer)

### `POST /bills/quote` — user
No database writes.

```json
{ "shopId": "…", "cartId": "crt_…", "addressId": "…", "slotId": "…optional…", "lines": [ { "itemId": "…shopProductId…", "subPref": "SIMILAR" } ] }
```

Response (also the shape of a submitted order's money part):

```json
{
  "lines": [ { "shopProductId": "…", "name": "Samba Rice 5kg", "unit": "PCS", "qty": 1, "unitPriceCents": 143500, "lineCents": 143500,
               "status": "OK", "priceChanged": false, "subPref": "SIMILAR" } ],
  "excluded": [ { "shopProductId": "…", "name": "…", "reason": "ITEM_UNAVAILABLE" } ],
  "totals": { "itemsCents": 143500, "deliveryFeeCents": 15000, "serviceFeeCents": 0, "discountCents": 0, "totalCents": 158500 },
  "delivery": { "distanceKm": 2.4, "weightKg": 5.0, "feeRule": "base + per km" },
  "slots": [ { "id": "…", "startsAt": "…", "endsAt": "…", "remaining": 4 } ],
  "checks": { "shopOpen": true, "inRange": true, "minOrderMet": true, "phoneVerified": false }
}
```

### `POST /bills` — user, `Idempotency-Key` required
Body: quote body + `"slotId"` (required for `SLOT`), `"note"`.
- Re-runs every check (`FR-BILL-03`). Returns `201` with the **order detail** (below).
- Errors: `PHONE_NOT_VERIFIED`, `TOO_MANY_OPEN_ORDERS`, `SHOP_CLOSED`, `SHOP_NOT_READY`, `OUT_OF_RANGE`, `BELOW_MIN_ORDER`, `SLOT_FULL`, `SLOT_CLOSED`, `ITEM_UNAVAILABLE`, `QTY_LIMIT`, `CART_EMPTY`, `CART_WRONG_SHOP`, `CART_UNAVAILABLE`, `RATE_LIMITED`.

### Order detail object
Returned by every order endpoint (customer, console, driver), with fields hidden by viewer.

```json
{
  "id": "…", "code": "KS-7Q2M9X", "status": "AWAITING_PAYMENT", "version": 4, "revisionNo": 1,
  "shop": { "id": "…", "slug": "krishna-super", "name": "Krishna Super", "phone": "+94…" },
  "slot": { "startsAt": "…", "endsAt": "…" },
  "address": { "label": "Home", "line1": "…", "landmark": "…" },
  "lines": [ { "id": "…", "name": "…", "unit": "PCS", "qty": 2, "unitPriceCents": 0, "lineCents": 0, "state": "ACTIVE", "subPref": "REMOVE", "revisionNo": 1, "replacesItemId": null } ],
  "previousLines": [ ],
  "totals": { "itemsCents": 0, "deliveryFeeCents": 0, "serviceFeeCents": 0, "discountCents": 0, "totalCents": 0 },
  "coupon": { "code": "WELCOME10", "discountCents": 0 },
  "payment": { "provider": "PAYHERE", "status": "INITIATED" },
  "deliveryCode": null,
  "expiresAt": "…",
  "timeline": [ { "status": "SUBMITTED", "at": "…" } ],
  "allowedActions": [ "PAY", "APPLY_COUPON", "CANCEL" ]
}
```

- `allowedActions` is computed **on the server** from the transition table + viewer. The UI shows buttons only from this list. Values: `CANCEL`, `ACCEPT_REVISION`, `APPLY_COUPON`, `REMOVE_COUPON`, `PAY`, `REPORT_PROBLEM`, console: `APPROVE`, `REVISE`, `REJECT`, `START_PICKING`, `MARK_PACKED`, `ASSIGN_DRIVER`, `SEND_OUT`, `RETRY_DELIVERY`, `REQUEST_CANCEL`, `REQUEST_PARTIAL_REFUND`, driver: `COMPLETE`, `FAIL`.
- `deliveryCode` is present **only** for the customer and only in `OUT_FOR_DELIVERY`.
- Console viewers also get `customer: { name, phoneMasked }` and, for picking, `phone` (full) — needed to call.
- Drivers get `customer: { name, phone }`, address with `lat`/`lng`, `totalCents`, no prices per line.

| Method | Path | Auth | Body | Notes |
|---|---|---|---|---|
| `GET` | `/orders?cursor=` | user | — | Own orders, newest first, summary objects |
| `GET` | `/orders/{id}` | user (own) | — | Order detail |
| `POST` | `/orders/{id}/accept-revision` | user (own) | `{ "version": 3, "revisionNo": 1 }` | T07 |
| `POST` | `/orders/{id}/cancel` | user (own) | `{ "version": 3 }` | T06 / T09 / T12 |
| `POST` | `/orders/{id}/coupon` | user (own) | `{ "version": 4, "code": "WELCOME10" }` | Coupon errors from `error_codes` |
| `DELETE` | `/orders/{id}/coupon` | user (own) | `{ "version": 5 }` | |
| `POST` | `/orders/{id}/payment` | user (own), `Idempotency-Key` | `{ "version": 5 }` | See below |
| `POST` | `/orders/{id}/tickets` | user (own) | `{ "reason": "MISSING_ITEM", "note": "…" }` | Paid orders only |

### `POST /orders/{id}/payment` — response

PayHere:

```json
{
  "provider": "PAYHERE",
  "action": "https://sandbox.payhere.lk/pay/checkout",
  "fields": {
    "merchant_id": "…", "return_url": "https://www.<domain>/order/…?paid=1", "cancel_url": "https://www.<domain>/order/…",
    "notify_url": "https://api.<domain>/v1/webhooks/payhere",
    "order_id": "pay_…", "items": "Order KS-7Q2M9X", "currency": "LKR", "amount": "1585.00",
    "first_name": "Nimal", "last_name": "-", "email": "…", "phone": "+94…", "address": "…", "city": "…", "country": "Sri Lanka",
    "hash": "…"
  }
}
```

The browser builds a hidden form with these fields and submits it. `order_id` is the **payment id** (one per attempt).

Manual:

```json
{ "provider": "MANUAL", "bank": { "bankName": "…", "accountName": "…", "accountNumber": "…", "branch": "…" }, "reference": "KS-7Q2M9X", "amountCents": 158500 }
```

Errors: `PAYMENT_NOT_AVAILABLE`, `INVALID_TRANSITION`, `STALE_ORDER`.

---

## 7. Webhooks

### `POST /webhooks/payhere` — sig
`application/x-www-form-urlencoded` from PayHere. Steps in `09-integrations.md` §2. Always answers `200` quickly (even when ignored), except a bad signature → `400` with no detail.

### `POST /connectors/{id}/trigger` — sig
Called by the shop's own system after a price or stock change.

Headers:
- `X-GP-Timestamp: 1730541000` (unix seconds, within 5 min)
- `X-GP-Signature: v1=<hex HMAC-SHA256 of "<timestamp>.<raw body>" with the trigger secret>`

Body (optional): `{ "externalIds": ["12345", "12346"] }` — up to 100. Without it, an incremental run.
Response `202 { "queued": true }`. Rate limited.

---

## 8. Console — shop (`/console/shops/{shopId}/…`)

The middleware loads the caller's **active membership for `{shopId}`** (or admin), then sets `app.shop_id`. No membership → `404 NOT_FOUND` (does not reveal the shop exists).

| Method | Path | Permission | Body / query | Notes |
|---|---|---|---|---|
| `GET` | `/orders?status=&cursor=` | `order.review` | `status` comma list | Summary list, `ETag` |
| `GET` | `/orders/{orderId}` | `order.review` | — | Order detail (console view) |
| `POST` | `/orders/{orderId}/review` | `order.review` | see below | T02 / T03 / T04 |
| `POST` | `/orders/{orderId}/status` | `order.fulfil` | `{ "version", "to": "PICKING" \| "PACKED" \| "OUT_FOR_DELIVERY" }` | T13 / T14 / T15 / T18 |
| `POST` | `/orders/{orderId}/driver` | `order.assign_driver` | `{ "version", "driverId" }` | Driver must be allowed by `driver_source` |
| `POST` | `/orders/{orderId}/refund-requests` | `order.request_refund` | `{ "type": "FULL" \| "PARTIAL", "orderItemId", "reason" }` | |
| `GET` | `/drivers` | `order.assign_driver` | — | Shop drivers (+ platform drivers if allowed) |
| `GET` | `/products?q=&cursor=&available=` | `order.review` | — | The shop's own products by name, 50 per page → `{ "data": [ { id, productId, name, brand, sizeLabel, barcode, imageUrl, shopPriceCents, mrpCents, onlinePriceCents, stockQty, isAvailable, maxPerOrder, managedBy, updatedAt } ], "nextCursor": "" }`. `available` is `true` or `false`; `q` is a word of the name, brand or keywords. `stockQty` is a number or `null`. |
| `PATCH` | `/products/{shopProductId}` | `product.edit` | `{ shopPriceCents, mrpCents, stockQty, isAvailable, maxPerOrder }`, any of them | Returns the shop product. A field left out keeps its value; `mrpCents: null` removes the MRP, `stockQty: null` stops tracking stock; `stockQty` is a whole number in phase 1. The customer price is worked out by the server (docs/07 section 6) and is never accepted. For a **connector-managed** product `shopPriceCents`, `mrpCents`, `stockQty` and `isAvailable` → `400 VALIDATION_ERROR` (`fields.<name> = "managed_by_connector"`, all of them named); `maxPerOrder` is always editable. Prices that leave no valid customer price → `fields.shopPriceCents` or `fields.mrpCents` = `invalid_price`. A product of another shop is `404`. Price changes are written to `price_changes` and `audit_log`. |
| `GET` | `/team` | `team.manage` | — | Members + pending invites |
| `POST` | `/team/invites` | `team.manage` | `{ email, role: "SHOP_STAFF" \| "SHOP_DRIVER", canEditProducts }` | Owner cannot create owners (`400`). `201 { invite, token }`. The **token is shown once** (only its hash is stored); there is no e-mail sender, so the console shows the invite link to the owner to pass on (the sign-in page sends the token as `inviteToken`). A new invite for the same e-mail replaces the old one. |
| `PATCH` | `/team/{membershipId}` | `team.manage` | `{ role, canEditProducts }` (both optional, one needed) | `204`. Cannot change own role or an owner (`403`); role only `SHOP_STAFF` / `SHOP_DRIVER`; the flag is cleared for drivers |
| `DELETE` | `/team/{membershipId}` | `team.manage` | — | `204`. Cannot remove last owner (`409 CONFLICT`). Takes effect on the next request |
| `GET` | `/dashboard?from=&to=` | `dashboard.shop_ops` | dates (Colombo) | Full numbers only with `dashboard.shop_full` |
| `GET` | `/payouts` | `dashboard.shop_full` | — | Balance due + payout history |

**Review body:**

```json
{
  "version": 2,
  "action": "REVISE",
  "reason": null,
  "slotId": null,
  "changes": [
    { "itemId": "…", "qty": 1 },
    { "itemId": "…", "remove": true },
    { "itemId": "…", "substitute": { "shopProductId": "…", "qty": 2 } }
  ]
}
```

- `APPROVE`: no `changes`. `REJECT`: `reason` required (3–300 chars). `REVISE`: at least one change.
- Substitutes must be from the same shop, available, and priced by the server.

---

## 9. Driver (`/driver/…`)

| Method | Path | Permission | Body | Notes |
|---|---|---|---|---|
| `GET` | `/deliveries?day=YYYY-MM-DD` | `order.deliver` | — | Orders where `driver_id = me`, grouped by slot |
| `POST` | `/deliveries/{orderId}/complete` | `order.deliver` | `{ "version", "code": "4821" }` | T16. Errors `DELIVERY_CODE_INVALID`, `DELIVERY_CODE_LOCKED` |
| `POST` | `/deliveries/{orderId}/fail` | `order.deliver` | `{ "version", "reason" }` | T17 |
| `GET` | `/stats?from=&to=` | `dashboard.driver` | — | Counts only |

---

## 10. Admin (`/admin/…`) — all `admin`

| Area | Method and path | Body / notes |
|---|---|---|
| Shops | `GET /shops`, `POST /shops`, `GET /shops/{id}`, `PATCH /shops/{id}` | Profile fields. Create, or a change to slug / name / logo / status, adds a `site.rebuild` event (D30). |
| | `PUT /shops/{id}/settings` | All `shop_settings` fields (validated against enums) |
| | `PUT /shops/{id}/hours` | `{ "days": [ { "weekday": 1, "opens": "08:00", "closes": "21:00" } ] }`. Replaces the whole week. `weekday` is ISO (1 = Monday … 7 = Sunday). Times are Colombo time. `"24:00"` is allowed only as `closes`; the closing minute itself is closed. A day with no row is closed. |
| | `POST /shops/{id}/accepting` | `{ "on": true }` — refused with `SHOP_NOT_READY` if not ready; `error.fields` has one entry per missing item (`{ "PAYMENT_ACCOUNT": "missing" }`). Turning off always works. |
| | `POST /shops/{id}/cart-tenant` | Creates the cart service tenant (platform key), saves tenant id + public key |
| | `GET /shops/{id}/readiness` | `{ "ready": false, "missing": ["PAYMENT_ACCOUNT", "FUTURE_SLOT"] }`. Items: `SHOP_ACTIVE`, `HOURS`, `PAYMENT_ACCOUNT`, `FUTURE_SLOT` (only for `SLOT` delivery). |
| Payment accounts | `GET /payment-accounts`, `POST /payment-accounts`, `PATCH /payment-accounts/{id}` | `merchantSecret` is **write-only** → stored in SSM at `payhere/<accountId>`, never returned, never written to the audit log (only `secretRotated: true`). `MANUAL` accounts take no credentials. |
| Fee rules | `GET /fee-rules`, `PUT /fee-rules/default`, `PUT /fee-rules/shops/{id}`, `POST /fee-rules/preview` | `GET` → `{ "data": [ { shopId, rule, updatedAt } ] }`, default first (`shopId: null`). `PUT` replaces the whole rule; a field left out keeps its default. Rule: `{ baseCents, perKmCents, freeKm, perKgCents, freeKg, freeAboveCents, minCents, maxCents, maxKm, roadFactor }`. `freeKm`, `freeKg`, `maxKm`, `roadFactor` are JSON numbers with at most 2 decimals (`maxKm` 0.01–50, `roadFactor` 1.00–2.00). A shop uses its own rule, else the default. Preview: `{ shopId, distanceKm, weightKg, itemsCents }` (distance is the straight line) → `{ feeCents, rule: "SHOP" \| "DEFAULT" }`; past `maxKm` → `409 OUT_OF_RANGE`; no rule at all → `409 SHOP_NOT_READY`. Every save is audited (`fee_rule.set`). |
| Slots | `GET /shops/{id}/slots?from=&to=`, `POST /shops/{id}/slots`, `PATCH /slots/{id}`, `POST /shops/{id}/slots/copy-week` | `from` / `to` are Colombo dates (`2026-11-02`), both inclusive; default the next 7 days, longest range 31 days → `{ "data": [ slot ] }` with `booked`. Create body: `{ "slots": [ { startsAt, endsAt, cutoffAt, capacity } ] }` (1–50, all or nothing): start in the future and within 90 days, length ≤ 24 h, `cutoffAt` ≤ `startsAt`, capacity 0–500; a start the shop already has → `409 CONFLICT`. `PATCH` body: any of `{ capacity, status: "OPEN" \| "CLOSED", cutoffAt }`; capacity may go below `booked` (the slot shows full, orders stay). Copy-week body: `{ "weekStart": "2026-11-02" }` copies the 7 days from that Colombo date to the next 7 days as empty, open slots, skips starts that already exist (safe to repeat) → `{ created, slots }`. Changes are audited (`slot.create`, `slot.update`, `slot.copy_week`). |
| Catalog | `GET/POST /categories`, `PATCH /categories/{id}` | Category: `{ id, parentId, slug, name, sort, defaultWeightG }`. `GET` → `{ "data": [ … ] }` ordered by `sort`, `name`. Create needs `slug` (lowercase words with `-`, ≤ 60) and `name`; `sort` defaults to 0 and `defaultWeightG` to 500 (1–100000). Two levels only: a parent must be a top-level category, and a category with children cannot get a parent (`fields.parentId` = `invalid` / `has_children`). `PATCH` takes any field, `parentId: null` makes it top level. Slug taken → `fields.slug = "taken"`. Audited (`category.create`, `category.update`). |
| | `GET/POST /products`, `PATCH /products/{id}` | Master product: `{ id, barcode, name, brand, sizeLabel, unit, weightG, categoryId, imageUrl, keywords, status }`. Create needs only `name` (2–120); `unit` defaults to `PCS`, `status` to `ACTIVE`. `barcode` is 6–14 digits and unique (`fields.barcode = "taken"`). `imageUrl` must be `https://` and under `IMG_BASE_URL` (`fields.imageUrl = "invalid"` for anything else, including `javascript:` and other hosts). A `null` clears `barcode`, `brand`, `sizeLabel`, `weightG`, `categoryId`, `imageUrl`. `GET ?q=&cursor=` → `{ "data": [ … ], "nextCursor": "" }`, 50 per page by name; `q` is a word of the name, brand or keywords. Audited (`product.create`, `product.update`). |
| | `POST /shops/{id}/products` | Puts a master product on sale in a shop by hand (`managedBy = MANUAL`): `{ productId, shopPriceCents, mrpCents?, stockQty?, isAvailable?, maxPerOrder? }` → `201` shop product (same shape as the console list). `isAvailable` defaults to `true`, `maxPerOrder` to 20. Already linked → `fields.productId = "taken"`; unknown product → `fields.productId = "unknown"`. Shop prices later change through the console `PATCH /products/{shopProductId}` (an admin may use it with the shop id). Audited (`shop_product.create`). |
| | `GET/PUT /shops/{id}/category-markups` | `{ "markups": [ { categoryId, markupBp } ] }` (`markupBp` 0–2000, at most 500, each category once). `PUT` replaces the whole list and reprices every product of the shop in the same transaction (`price_changes.source = MARKUP`); a refused list changes nothing. Both answer `{ "data": [ … ] }`. Audited (`category_markups.set`). A change of the shop markup in `PUT /shops/{id}/settings` reprices the same way. |
| Connector | `POST /shops/{id}/connector` | `{ "connectionString": "postgresql://…" }` → saved to SSM, returns connector (masked host) |
| | `PUT /connectors/{id}/secret` | Replace connection string |
| | `POST /connectors/{id}/test` | `{ ok, readOnly, serverVersion, warnings: [] }` or `CONNECTOR_TEST_FAILED` |
| | `GET /connectors/{id}/tables` · `GET /connectors/{id}/columns?table=` | From `information_schema` |
| | `PUT /connectors/{id}/mapping` | `{ sourceTable, moneyUnit, changeColumn, intervalMin, autoCreateProducts, columns: { externalId: "…", barcode, name, … } }` |
| | `POST /connectors/{id}/preview` | First 20 mapped rows + what would happen (`create` / `update` / `unmatched` / `hold`) |
| | `POST /connectors/{id}/activate` · `/pause` · `/sync` | `/sync` → `202` or `CONNECTOR_BUSY` |
| | `POST /connectors/{id}/trigger-secret` | Rotates; returns secret **once** + example code |
| | `GET /connectors/{id}/runs` | Last 50 runs |
| Sync issues | `GET /sync-issues?status=OPEN`, `POST /sync-issues/{id}/resolve` | `{ "action": "LINK", "productId" }` · `CREATE` · `APPROVE_PRICE` · `REJECT_PRICE` · `IGNORE` |
| Coupons | `GET /coupons`, `POST /coupons`, `PATCH /coupons/{id}` | Code upper-cased on save |
| Orders | `GET /orders?shopId=&status=&q=` | `q` = order code or phone last 4 |
| | `POST /orders/{id}/transition` | `{ version, to, reason }` — admin transitions only |
| | `POST /orders/{id}/mark-paid` | `{ version, reference }` — manual payments (T10) |
| | `POST /orders/{id}/unlock-code` | Resets attempts |
| | `POST /orders/{id}/driver` | Assign platform driver |
| Refunds | `GET /refunds?status=`, `POST /refunds/{id}/decide` (`{ approve, note }`), `POST /refunds/{id}/paid-out` (`{ reference }`) | |
| Tickets | `GET /tickets?status=`, `POST /tickets/{id}/resolve` | |
| Payouts | `GET /payouts?shopId=`, `POST /payouts` (`{ shopId, from, to }` → DRAFT with amount from ledger), `POST /payouts/{id}/paid` (`{ reference }`) | |
| People | `GET /users?q=`, `PATCH /users/{id}` (`{ status }`), `POST /invites` (`{ email, role, shopId }`), `DELETE /invites/{id}` | `q` is a prefix of e-mail or name (max 50 rows, newest first). `status` is `ACTIVE` or `BLOCKED`; blocking ends every session; an admin cannot block themselves (`403`). Invites: `SHOP_OWNER` / `SHOP_STAFF` / `SHOP_DRIVER` need `shopId`, `PLATFORM_DRIVER` must not have one; `201 { invite, token }` (token shown once); revoking an unused invite is `204`, otherwise `404` |
| Insight | `GET /dashboard?from=&to=`, `GET /notifications/usage?from=&to=`, `GET /audit?entity=&entityId=&cursor=` | Audit: `{ "data": [ { id, actorId, actorRole, action, entity, entityId, shopId, before, after, at } ], "nextCursor": "" }`, newest first, 50 per page. Secret-looking keys in `before` / `after` are masked. |

## 11. Uploads

### `POST /uploads/presign` — `admin` or `product.edit` (with `shopId`)
Body: `{ "purpose": "PRODUCT_IMAGE" | "SHOP_LOGO", "contentType": "image/webp" | "image/jpeg" | "image/png", "size": 183000, "shopId": "…" }`
- `size` ≤ 2 MB (`2097152`). Any other `contentType` (SVG, HTML, GIF) or a bigger size → `400 UPLOAD_REJECTED`. A `size` that is not a positive number, or an unknown `purpose` → `400 VALIDATION_ERROR`. Key is generated by the server: `<purpose>/<uuid>.<ext>`.
- Who: without `shopId` the caller must be the admin (TOTP passed); with `shopId` the caller needs `product.edit` in that shop (no membership there → `404`, no right → `403`). `SHOP_LOGO` is admin only (`shopId` with it → `fields.purpose = "not_allowed"`).
- Returns `{ "uploadUrl": "…", "publicUrl": "https://img.<domain>/…", "expiresIn": 300 }`. The presigned URL is locked to the content type and length.

## 12. Route → handler map

All routes are registered in **one** file: `backend/internal/http/routes.go`. Each line names: method, path, auth level, permission, handler. A test checks that every route in this document exists there and nothing else does.
