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
- `lat` and `lng` are optional but come together, inside Sri Lanka (else `400`, `fields.lat` / `fields.lng` = `required`, `invalid` or `out_of_range`). The **server rounds them to 2 decimals** before measuring anything (about 1 km): the exact place is never used, and one cached answer (`max-age=60`) serves a whole neighbourhood.
- `shops`: with a pin, the active shops that deliver there (the rule of `deliveringShops` in §5), nearest first, each with `distanceKm` (straight line, 2 decimals). Without a pin, every active shop by name and `distanceKm: null`. `openNow` follows the weekly hours in Colombo time. `nextSlot` is the start of the soonest slot that can still be booked; it is left out when there is none.
- `deals`: up to 12 available products of those shops sold under their MRP, biggest `saveCents` first.
- `promos`: always `[]` for now (there is no promo data yet).

```json
{
  "shops": [ { "id": "…", "slug": "krishna-super", "name": "Krishna Super", "logoUrl": "…", "themeColor": "#0F5132", "openNow": true, "nextSlot": "2026-11-02T08:30:00Z", "distanceKm": 1.8 } ],
  "deals": [ { "shopProductId": "…", "shopSlug": "krishna-super", "shopName": "Krishna Super", "name": "Samba Rice 5kg", "imageUrl": "…", "onlinePriceCents": 143500, "mrpCents": 150000, "saveCents": 6500 } ],
  "promos": [ { "shopSlug": "krishna-super", "title": "Weekend offers", "imageUrl": "…" } ]
}
```

### `GET /search?q=&shop=&cursor=&lat=&lng=` — public, cacheable (30 s)
- `q` 2–60 characters after trimming (else `400`, `fields.q`). The text is only ever a query parameter, so odd text (`%' OR 1=1 --`) is just an empty result. It matches name, brand and keywords, and tolerates typos (`basmti` finds Basmati): `$q <% search_doc` with `pg_trgm.word_similarity_threshold = 0.4` set for the transaction.
- `shop` is an optional slug of an active shop (unknown or not active → `404`). Then only that shop is searched and `shops` is empty.
- Without `shop`: products across the shops that deliver to `lat,lng` (optional, same rules as `/home`, not rounded), or across every active shop without a pin. `shops` lists those shops whose name contains the text.
- Order: available products first, then cheapest first. 24 per page; `nextCursor` is `null` on the last page. Hidden products and products of shops that are not active are never listed. Each product carries `shopSlug`.

```json
{ "shops": [ … ], "products": { "data": [ { "shopProductId": "…", "shopSlug": "…", "name": "…", "onlinePriceCents": 0, "available": true } ], "nextCursor": null } }
```

### `GET /shops?fields=slug` — public, cacheable
Active shops: `{ "data": [ { "slug": "krishna-super", "name": "Krishna Super", "logoUrl": "…", "updatedAt": "…" } ] }`. Used by the web build to pre-render shop pages.

### `GET /shops/{slug}` — public, cacheable (60 s)
Only an `ACTIVE` shop; draft, suspended and unknown shops are all `404`.

```json
{
  "id": "…", "slug": "krishna-super", "name": "Krishna Super", "logoUrl": "…", "themeColor": "#0F5132",
  "openNow": true, "acceptingOrders": true, "hoursToday": { "opens": "08:00", "closes": "21:00" },
  "deliveryType": "SLOT", "minOrderCents": 100000,
  "cart": { "tenantId": "ten_…", "publicKey": "pk_live_…" },
  "categories": [ { "id": "…", "parentId": null, "slug": "rice", "name": "Rice", "count": 42 } ]
}
```

- `acceptingOrders` is the shop's switch. `hoursToday` is `null` on a day the shop is closed. `cart` is `null` until the shop has a cart tenant. Nothing else from the shop's settings is shown.
- `categories`: those the shop has products in, ordered by `sort` then name. `count` is the shop's visible products in the category; a top-level category also counts its children.

### `GET /shops/{slug}/products?category=&q=&cursor=&limit=` — public, cacheable (60 s)
`category` is a category slug (its children are included; an unknown slug gives an empty list). `q` is optional, 2–60 characters, with the same typo search as `/search`. `limit` is 1–50, default 24. Ordered by product name. Hidden products are left out; products that cannot be bought now are listed with `available: false`. `nextCursor` is `null` on the last page.

```json
{ "data": [ { "shopProductId": "…", "productId": "…", "name": "Samba Rice", "brand": "Araliya", "sizeLabel": "5kg", "unit": "PCS",
  "imageUrl": "…", "onlinePriceCents": 143500, "mrpCents": 150000, "available": true, "maxPerOrder": 20 } ], "nextCursor": "…" }
```

### `GET /shops/{slug}/slots?days=3` — public
Open slots for the next `days` (1–7, default 3; today counts as day 1, Colombo time) with `remaining = capacity − booked` (never below 0) and `cutoffAt`.
- Only slots that are `OPEN` and before their cutoff are listed. A full slot is listed with `remaining: 0`, so the page can show "full".
- Not cached (the numbers change with every order). Unknown or not-active shop → `404`.
- `{ "data": [ { "id": "…", "startsAt": "…", "endsAt": "…", "cutoffAt": "…", "remaining": 3 } ] }`

### `GET /categories` — public, cacheable (5 min)
Category tree: `{ "data": [ { "id": "…", "slug": "rice", "name": "Rice", "children": [ { "id": "…", "slug": "basmati", "name": "Basmati", "children": [] } ] } ] }`, ordered by `sort` then name.

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
| `GET` | `/shops/{slug}/buy-again` | user | — | `{ "data": [ product card ] }`: what the caller bought in this shop before, latest purchase first, at most 20, with today's `onlinePriceCents` and `available` (same card as the shop product list). Never cached. Unknown or not-active shop → `404`. |

---

## 6. Bills and orders (customer)

### `POST /bills/quote` — user
No database writes. The quantities are the ones in the cart; the body only chooses what to do when a line is not available. A body with a quantity, a price or any other unknown field is `400`. `lines` may be left out; every cart line is then `SIMILAR`. An `itemId` that is not in the cart is `400`.

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

Notes:
- `slots` lists the open windows of the next 7 days; a full one has `remaining: 0` and is still listed. Each also has `cutoffAt`.
- `slotId` is optional. If it is sent it is checked: another shop's slot is `404`; closed or past its cutoff is `SLOT_CLOSED`; full is `SLOT_FULL`.
- A line that is not for sale is in `excluded` (`ITEM_UNAVAILABLE`) and is not in any total. Another shop's product has no name there.
- Out of range gives `checks.inRange: false` and a delivery fee of 0, so the page can still show the bill.
- Errors: `CART_EMPTY`, `CART_WRONG_SHOP`, `CART_UNAVAILABLE`, `QTY_LIMIT`, `SLOT_FULL`, `SLOT_CLOSED`, `VALIDATION_ERROR`. A shop that is not active, and an address of someone else, are `404`.

### `POST /bills` — user, `Idempotency-Key` required
Body: quote body + `"slotId"` (required for `SLOT`), `"note"`.
- Re-runs every check (`FR-BILL-03`) and prices again from the database and a fresh read of the cart. Returns `201` with the **order detail** (below). The `Idempotency-Key` header is required (16–64 characters of `A-Z a-z 0-9 _ -`); a missing or badly shaped key is `400` with `fields["Idempotency-Key"]`.
- The key is saved in the same transaction as the order, so a refused bill leaves no key and may be sent again. A retry with the same key and body gets the stored order back (`201`), even though the cart is gone by then; the same key with another body is `409 IDEMPOTENCY_MISMATCH`. Keys belong to one customer and are kept 24 hours.
- `slotId` is required when the shop delivers by `SLOT` and refused (`400`, `fields.slotId = "not_used"`) when it does not. `note` is at most 300 characters.
- A cart line that is not for sale makes the whole bill `ITEM_UNAVAILABLE`: the customer fixes the cart and sends again. The quote shows such lines as `excluded`.
- Limits: fewer than 3 orders in `SUBMITTED`, `REVISED` or `AWAITING_PAYMENT` (`TOO_MANY_OPEN_ORDERS`) and 10 bills an hour; every attempt counts toward the hour, also a refused one (`RATE_LIMITED`).
- After the commit the cart is deleted; if that fails the cart service expires it.
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

- Another customer's order, a missing id and a malformed id are all `404`. `lines` are the current revision; earlier revisions are in `previousLines`. `expiresAt` is when the wait the order is in (review, revision or payment) runs out; it is left out of final statuses. `deliveryCode` is not built yet (it comes with the fulfilment prompt).
- The action names are code constants in `internal/order/actions.go` (they are not in `contracts/enums.json` yet).
- `allowedActions` is computed **on the server** from the transition table + viewer. The UI shows buttons only from this list. Values: `CANCEL`, `ACCEPT_REVISION`, `APPLY_COUPON`, `REMOVE_COUPON`, `PAY`, `REPORT_PROBLEM`, console: `APPROVE`, `REVISE`, `REJECT`, `START_PICKING`, `MARK_PACKED`, `ASSIGN_DRIVER`, `SEND_OUT`, `RETRY_DELIVERY`, `REQUEST_CANCEL`, `REQUEST_PARTIAL_REFUND`, driver: `COMPLETE`, `FAIL`.
- `deliveryCode` is present **only** for the customer and only in `OUT_FOR_DELIVERY`.
- Console viewers also get `customer: { name, phoneMasked }` and, for picking, `phone` (full) — needed to call.
- Drivers get `customer: { name, phone }`, address with `lat`/`lng`, `totalCents`, no prices per line.

| Method | Path | Auth | Body | Notes |
|---|---|---|---|---|
| `GET` | `/orders?limit=&cursor=` | user | — | Own orders, newest first, 20 per page (`limit` up to 50) → `{ "data": [ { id, code, status, shop: { slug, name }, totalCents, itemCount, createdAt } ], "nextCursor": "…" \| null }`. Never lists another customer's orders. |
| `GET` | `/orders/{id}` | user (own) | — | Order detail |
| `POST` | `/orders/{id}/accept-revision` | user (own) | `{ "version": 3, "revisionNo": 1 }` | T07. Returns the order detail. A `revisionNo` that is not the order's is `409 STALE_ORDER`; both numbers are at least 1 (`400`). The bill is frozen and the payment timer starts. |
| `POST` | `/orders/{id}/cancel` | user (own) | `{ "version": 3 }` | T06 / T09 / T12. Returns the order detail. Allowed in `SUBMITTED`, `REVISED` and `AWAITING_PAYMENT`; from `PAID` on it is `409 INVALID_TRANSITION` (BIZ-10). The slot place is released. |
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
Response `202 { "queued": true }`. Rate limited: 30 calls an hour per connector, counted only for calls with a good signature, so wrong guesses cannot use up the shop's hour. The time, signature, unknown connector and "no secret yet" all answer the same `401 UNAUTHENTICATED`; past the limit `429 RATE_LIMITED`. Body errors (more than 100 ids, an empty id, unknown field) → `400`. A connector that is not live (`ACTIVE` / `FAILING`) → `409 CONFLICT`, `fields.status = "not_active"`. The run itself starts in the sync function; if a run is already going, the call is recorded as `SKIPPED` and the next scheduled run catches up.

---

## 8. Console — shop (`/console/shops/{shopId}/…`)

The middleware loads the caller's **active membership for `{shopId}`** (or admin), then sets `app.shop_id`. No membership → `404 NOT_FOUND` (does not reveal the shop exists).

| Method | Path | Permission | Body / query | Notes |
|---|---|---|---|---|
| `GET` | `/orders?status=&limit=&cursor=` | `order.review` | `status` is a comma list of order statuses (a name that does not exist is `400`) | The shop's orders, newest first, 20 per page (`limit` up to 50) → `{ "data": [ { id, code, status, version, customer: { name, phoneMasked }, slot: { startsAt, endsAt } \| null, totalCents, itemCount, createdAt } ], "nextCursor": "…" \| null }`. Sends an `ETag`; a request with `If-None-Match` of that tag gets `304` and no body. `itemCount` counts the lines that still count (not removed ones). The shop is the one in the path: another shop's orders are never listed. |
| `GET` | `/orders/{id}` | `order.review` | — | Order detail (console view). An order of another shop is `404`, also for an admin on this shop's path. |
| `POST` | `/orders/{id}/review` | `order.review` | see below | T02 / T03 / T04. Returns the order detail. |
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

- `APPROVE`: no `changes`, no `reason`, no `slotId`. `REJECT`: `reason` required (3–300 chars), no `changes`, no `slotId`. `REVISE`: at least one change (at most 100); `reason` is an optional note (3–300 chars); `slotId` is optional. A field that does not belong to the action is `400`, not ignored. `version` is required (at least 1).
- Each change names one line of the current bill (`itemId`) and exactly one of `qty` (1–999), `remove` or `substitute`. A line may be named once. A change that leaves everything as it was (`no_change`), one that removes every line (`removes_all`) and a line that is not in the bill (`unknown_item`) are `400` with `fields.changes`.
- Substitutes must be from the same shop, available, and priced by the server. A product that cannot be sold now (another shop's, hidden, out of stock) is `409 ITEM_UNAVAILABLE`; a quantity over the product's `maxPerOrder` is `QTY_LIMIT`.
- **Revise** makes revision `revisionNo + 1`: every line is copied into it (a removed one with `state: REMOVED` and no amount, a substitute as a new `SUBSTITUTE` line whose `replacesItemId` is the removed line), and the items, delivery fee, service fee and total are worked out again by the server from today's prices. The earlier revision stays in `previousLines`. With `slotId` the order moves to that window (booked before the old one is released; a full or closed slot is `SLOT_FULL` / `SLOT_CLOSED`, another shop's slot is `404`); a courier order takes no `slotId` (`400`, `not_used`). The revision timer starts.
- **Approve** freezes the bill (`frozen_at`) and starts the payment timer. **Reject** releases the slot place.
- A bill that is not `SUBMITTED` cannot be reviewed (`409 INVALID_TRANSITION`); a wrong `version` is `409 STALE_ORDER`, so of two reviews with the same version the second fails (BIZ-17).

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
| | `POST /shops/{id}/cart-tenant` | No body → `200` shop detail. Creates the shop's tenant in the cart service (platform key, `externalId` = shop id), saves `cartTenantId` and `cartPublicKey` (the secret key is never kept), audits `shop.cart_tenant.create` and queues the first full price upload. Safe to repeat: a shop that already has a tenant is returned as it is. Cart service down or refusing → `503 CART_UNAVAILABLE`, nothing saved. |
| | `POST /shops/{id}/cart-sync` | No body → `202 { "queuedItems": 120 }`. "Re-sync cart prices": queues every product of the shop for upload again. Cart not enabled → `409 CONFLICT` (`fields.cart = "not_enabled"`). |
| | `GET /shops/{id}/readiness` | `{ "ready": false, "missing": ["PAYMENT_ACCOUNT", "FUTURE_SLOT"] }`. Items: `SHOP_ACTIVE`, `HOURS`, `PAYMENT_ACCOUNT`, `FUTURE_SLOT` (only for `SLOT` delivery). |
| Payment accounts | `GET /payment-accounts`, `POST /payment-accounts`, `PATCH /payment-accounts/{id}` | `merchantSecret` is **write-only** → stored in SSM at `payhere/<accountId>`, never returned, never written to the audit log (only `secretRotated: true`). `MANUAL` accounts take no credentials. |
| Fee rules | `GET /fee-rules`, `PUT /fee-rules/default`, `PUT /fee-rules/shops/{id}`, `POST /fee-rules/preview` | `GET` → `{ "data": [ { shopId, rule, updatedAt } ] }`, default first (`shopId: null`). `PUT` replaces the whole rule; a field left out keeps its default. Rule: `{ baseCents, perKmCents, freeKm, perKgCents, freeKg, freeAboveCents, minCents, maxCents, maxKm, roadFactor }`. `freeKm`, `freeKg`, `maxKm`, `roadFactor` are JSON numbers with at most 2 decimals (`maxKm` 0.01–50, `roadFactor` 1.00–2.00). A shop uses its own rule, else the default. Preview: `{ shopId, distanceKm, weightKg, itemsCents }` (distance is the straight line) → `{ feeCents, rule: "SHOP" \| "DEFAULT" }`; past `maxKm` → `409 OUT_OF_RANGE`; no rule at all → `409 SHOP_NOT_READY`. Every save is audited (`fee_rule.set`). |
| Slots | `GET /shops/{id}/slots?from=&to=`, `POST /shops/{id}/slots`, `PATCH /slots/{id}`, `POST /shops/{id}/slots/copy-week` | `from` / `to` are Colombo dates (`2026-11-02`), both inclusive; default the next 7 days, longest range 31 days → `{ "data": [ slot ] }` with `booked`. Create body: `{ "slots": [ { startsAt, endsAt, cutoffAt, capacity } ] }` (1–50, all or nothing): start in the future and within 90 days, length ≤ 24 h, `cutoffAt` ≤ `startsAt`, capacity 0–500; a start the shop already has → `409 CONFLICT`. `PATCH` body: any of `{ capacity, status: "OPEN" \| "CLOSED", cutoffAt }`; capacity may go below `booked` (the slot shows full, orders stay). Copy-week body: `{ "weekStart": "2026-11-02" }` copies the 7 days from that Colombo date to the next 7 days as empty, open slots, skips starts that already exist (safe to repeat) → `{ created, slots }`. Changes are audited (`slot.create`, `slot.update`, `slot.copy_week`). |
| Catalog | `GET/POST /categories`, `PATCH /categories/{id}` | Category: `{ id, parentId, slug, name, sort, defaultWeightG }`. `GET` → `{ "data": [ … ] }` ordered by `sort`, `name`. Create needs `slug` (lowercase words with `-`, ≤ 60) and `name`; `sort` defaults to 0 and `defaultWeightG` to 500 (1–100000). Two levels only: a parent must be a top-level category, and a category with children cannot get a parent (`fields.parentId` = `invalid` / `has_children`). `PATCH` takes any field, `parentId: null` makes it top level. Slug taken → `fields.slug = "taken"`. Audited (`category.create`, `category.update`). |
| | `GET/POST /products`, `PATCH /products/{id}` | Master product: `{ id, barcode, name, brand, sizeLabel, unit, weightG, categoryId, imageUrl, keywords, status }`. Create needs only `name` (2–120); `unit` defaults to `PCS`, `status` to `ACTIVE`. `barcode` is 6–14 digits and unique (`fields.barcode = "taken"`). `imageUrl` must be `https://` and under `IMG_BASE_URL` (`fields.imageUrl = "invalid"` for anything else, including `javascript:` and other hosts). A `null` clears `barcode`, `brand`, `sizeLabel`, `weightG`, `categoryId`, `imageUrl`. `GET ?q=&cursor=` → `{ "data": [ … ], "nextCursor": "" }`, 50 per page by name; `q` is a word of the name, brand or keywords. Audited (`product.create`, `product.update`). |
| | `POST /shops/{id}/products` | Puts a master product on sale in a shop by hand (`managedBy = MANUAL`): `{ productId, shopPriceCents, mrpCents?, stockQty?, isAvailable?, maxPerOrder? }` → `201` shop product (same shape as the console list). `isAvailable` defaults to `true`, `maxPerOrder` to 20. Already linked → `fields.productId = "taken"`; unknown product → `fields.productId = "unknown"`. Shop prices later change through the console `PATCH /products/{shopProductId}` (an admin may use it with the shop id). Audited (`shop_product.create`). |
| | `GET/PUT /shops/{id}/category-markups` | `{ "markups": [ { categoryId, markupBp } ] }` (`markupBp` 0–2000, at most 500, each category once). `PUT` replaces the whole list and reprices every product of the shop in the same transaction (`price_changes.source = MARKUP`); a refused list changes nothing. Both answer `{ "data": [ … ] }`. Audited (`category_markups.set`). A change of the shop markup in `PUT /shops/{id}/settings` reprices the same way. |
| Connector | `POST /shops/{id}/connector`, `GET /shops/{id}/connector` | Body `{ "connectionString": "postgresql://user:pass@host/db?sslmode=require" }` → `201` connector `{ id, shopId, status: "DRAFT", host, sourceTable, moneyUnit, changeColumn, intervalMin, autoCreateProducts, columns, lastRunAt }`. `host` is masked (`postgres://user:•••@host/db`); the string itself goes to the secret store (`connectors/<id>`) and is never returned, logged or audited. One connector per shop (`409 CONFLICT`). Only `postgres://` or `postgresql://`; a password is required; `sslmode` may be `require`, `verify-ca` or `verify-full` (the server checks the certificate); `host`, `hostaddr`, `sslrootcert`, `options` and other keys are refused. Any refusal → `400 VALIDATION_ERROR`, `fields.connectionString = "invalid"`. `GET` returns the same connector, or `404`. Audited (`connector.create`). |
| | `PUT /connectors/{id}/secret` | Replaces the connection string, same checks. The mapping stays; the saved read position (`last_cursor`) is not touched. Audited (`connector.secret.replace`, no value). |
| | `POST /connectors/{id}/test` | `{ ok, readOnly, serverVersion, warnings: [] }`. Opens the connection in a read-only transaction. A login that can write still works, with a warning. Any failure (bad host, blocked address, wrong password, timeout) → `CONNECTOR_TEST_FAILED` with one fixed message; the cause is only in the server log as a stage and an SQLSTATE. Hosts that resolve to private, loopback, link-local or metadata addresses are refused (SSRF). |
| | `GET /connectors/{id}/tables` · `GET /connectors/{id}/columns?table=` | `{ "data": [ { schema, name, kind } ] }` and `{ "data": [ { name, type } ] }` from `information_schema`. `table` is `schema.name` or `name` (default schema `public`). |
| | `PUT /connectors/{id}/mapping` | `{ sourceTable, moneyUnit: "RUPEES" \| "CENTS", changeColumn, intervalMin (10–1440, default 60), autoCreateProducts, columns: { externalId, name, price, barcode, mrp, stock, available, brand, unit, weightG, category, imageUrl, updatedAt } }`. `externalId`, `name` and `price` are required. The table and every column must exist in the source; column names are plain lower-case identifiers. `changeColumn` must be a time column and, if `columns.updatedAt` is set, the same one (default: `updatedAt`). A refused mapping changes nothing: `400 VALIDATION_ERROR` with `fields.<key>` (`columns.price`, `sourceTable`, `changeColumn`, `intervalMin`, `moneyUnit`). Saving resets `last_cursor`. Audited (`connector.mapping.set`). |
| | `POST /connectors/{id}/preview` | Reads the first 20 rows by `externalId` and answers `{ "rows": [ { externalId, name, barcode, priceCents, mrpCents, stockQty, available, action, reason } ] }`. `action` is `create` / `update` / `unmatched` / `hold` / `invalid`; `reason` is `barcode`, `auto_create`, `no_barcode`, `unknown_barcode`, `already_linked`, `zero_price`, `bad_price`, `over_mrp`, `big_drop`, `no_name`, … `priceCents` is `null` when the price cannot be read. No mapping yet → `409 CONFLICT`, `fields.mapping = "not_set"`. Writes nothing in either database. |
| | `POST /connectors/{id}/activate` · `/pause` | Return the connector. Activate needs a saved mapping (`409 CONFLICT`, `fields.mapping = "not_set"`); it also clears the failure count. A live connector (`ACTIVE` or `FAILING`) has a recurring schedule `sync-<id>` at `rate(intervalMin minutes)`; pause removes it. A new `intervalMin` in the mapping moves the schedule of a live connector. A draft cannot be paused (`fields.status = "not_active"`). Audited (`connector.activate`, `connector.pause`). |
| | `POST /connectors/{id}/sync` | "Sync now". `202 { "queued": true }` and the run starts in the sync function, or `409 CONNECTOR_BUSY` when a run younger than 2 minutes is going. Works in any status once a mapping is saved, and at any hour. |
| | `POST /connectors/{id}/trigger-secret` | Rotates the HMAC secret for the trigger URL. `201 { secret, example }`: the secret is shown **once**; the example is a shell snippet that names it as `$TRIGGER_SECRET`. The old secret stops working at once. Audited (`connector.trigger_secret.rotate`, no value). |
| | `GET /connectors/{id}/runs` | `{ "data": [ { id, trigger, status, rowsRead, rowsChanged, rowsUnmatched, rowsHeld, error, startedAt, finishedAt } ] }`, last 50, newest first. `error` is one of `source_failed`, `apply_failed`, `timed_out`, never text from the shop's database. |
| Sync issues | `GET /sync-issues?status=&shopId=` | `{ "data": [ issue ] }`, newest first, at most 100. Issue: `{ id, connectorId, shopId, kind: "UNMATCHED" \| "PRICE_HOLD", externalId, payload, status: "OPEN" \| "RESOLVED" \| "IGNORED", createdAt, resolvedAt }`. `payload` is the cleaned source row: `{ reason, name, barcode, brand, unit, weightG, imageUrl, priceCents, mrpCents, stockQty, available, shopProductId?, currentPriceCents? }`. A bad `status` or `shopId` → `400`. |
| | `POST /sync-issues/{id}/resolve` | Body `{ action, productId? }`. `LINK` (`productId` required) and `CREATE` work on `UNMATCHED`: they sell the row as a new shop product (`managedBy = CONNECTOR`), `LINK` under an existing master product, `CREATE` after making one from the row. `APPROVE_PRICE` and `REJECT_PRICE` work on `PRICE_HOLD`: approve takes the held price (a price above the MRP drops the MRP; a held row for a product the shop does not sell yet is created); a price of 0 or one that cannot be read is never approved (`400`, `fields.action = "not_allowed"`). `IGNORE` works on both. The issue becomes `RESOLVED` (or `IGNORED`) in the same transaction as its effect. An issue that is not open → `409 CONFLICT`, `fields.status = "not_open"`. An action that does not fit the kind → `400 VALIDATION_ERROR`, `fields.action = "not_allowed"`. Audited (`sync_issue.resolve`). |
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
