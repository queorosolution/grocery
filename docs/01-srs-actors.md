# 01 — SRS: actor perspective

What each person (and outside system) can do. Requirement IDs (`FR-…`) are in `02-srs-features.md`.

## 1. Actors

| ID | Actor | Type | Scope |
|---|---|---|---|
| A1 | Guest | Human | Browse only |
| A2 | Customer | Human | Own account, own orders |
| A3 | Shop owner | Human | One shop (all of it) |
| A4 | Shop staff | Human | One shop (orders, picking, products if allowed) |
| A5 | Driver (shop or platform) | Human | Orders assigned to them |
| A6 | Super admin (you) | Human | Whole platform |
| S1 | Cart service | System | Holds cart lines |
| S2 | PayHere | System | Takes card / wallet payments |
| S3 | SMS / WhatsApp gateway | System | Sends messages |
| S4 | Google | System | Sign-in (OIDC), Maps (address pin) |
| S5 | Shop source database | System | Krishna Super's Neon DB, read only |
| S6 | Scheduler | System | Fires one-time timers and sync schedules |

A user can hold more than one role (a shop owner can also be a customer). Roles come from **memberships** (see `05-data-model.md`).

## 2. Role → permission summary

| Permission | A2 Customer | A3 Owner | A4 Staff | A5 Driver | A6 Admin |
|---|---|---|---|---|---|
| Browse, cart, bill, pay own order | ✅ | ✅ | ✅ | ✅ | ✅ |
| Review bills of the shop | | ✅ | ✅ | | ✅ |
| Move order to picking / packed | | ✅ | ✅ | | ✅ |
| Assign driver | | ✅ | ✅ | | ✅ |
| Edit shop product price / stock (manual products only) | | ✅ | if granted | | ✅ |
| Request cancel / partial refund | | ✅ | ✅ | | — |
| Approve cancel / refund | | | | | ✅ |
| Manage staff and shop drivers | | ✅ | | | ✅ |
| See shop dashboard | | ✅ (full) | ✅ (operations) | | ✅ |
| Complete delivery with OTP | | | | ✅ | ✅ |
| Shop settings, payment, delivery fees, slots, hours | | | | | ✅ |
| Connectors, master catalog, coupons, payouts | | | | | ✅ |

The exact map is `internal/shared/authz/permissions.go` (generated names from `contracts/enums.json`).

---

## 3. Use cases

### A1 Guest

| ID | Use case | Main flow | Notes |
|---|---|---|---|
| UC-GST-01 | Browse home | Opens site → sees search bar, shops that deliver near a chosen location, lowest-price items, promotions. | Location from browser (asked once) or typed town. |
| UC-GST-02 | Search | Types product and/or picks a shop in the 2-way search bar → results. | Product only = all shops, cheapest first. |
| UC-GST-03 | Open a shop | Sees categories, products, open/closed badge, delivery info. | Closed shop: browsing allowed, checkout blocked. |
| UC-GST-04 | Add to cart | Taps "Add" → line goes to the cart service. | Cart belongs to that shop. Adding from another shop asks "Clear cart and start a new one?" |
| UC-GST-05 | Sign in | Taps "Continue with Google" at checkout. | Cart stays (it is not tied to the login). |

### A2 Customer

| ID | Use case | Main flow | Other flows |
|---|---|---|---|
| UC-CUS-01 | First-time setup | After Google sign-in: name (pre-filled) → phone → address with map pin + landmark. 3 short screens. | Address outside every shop's range → "Not in your area yet". |
| UC-CUS-02 | Open bill page | Checkout → bill page shows items with **server prices**, delivery fee, service fee, total, slot picker, substitution choice per line, note. | Price changed since added → line shows "price updated". Item unavailable → line shows "not available" and is excluded. |
| UC-CUS-03 | Verify phone | First bill only: SMS code → enter → verified. | 5 wrong codes → wait 1 hour. |
| UC-CUS-04 | Submit bill | Picks slot → "Send to shop" → order `SUBMITTED`. Slot is held. Shop is notified. | Slot just became full → choose another. Shop closed → blocked. |
| UC-CUS-05 | Accept revision | Shop changed the bill → customer sees old vs new lines → "Accept" or "Cancel". | No answer before timeout → `EXPIRED`. |
| UC-CUS-06 | Apply coupon | After approval, on the payment screen → enters code → discount shown. | Invalid / expired / limit reached / below minimum → clear message. One coupon per order. |
| UC-CUS-07 | Pay | "Pay now" → PayHere page → back to order page. Order shows `PAID` only after the server receives the verified callback. | Payment failed → stays `AWAITING_PAYMENT`, can retry until timeout. Manual mode → bank details + reference shown. |
| UC-CUS-08 | Cancel before payment | "Cancel order" in `SUBMITTED`, `REVISED`, `AWAITING_PAYMENT`. | After payment the button is gone. |
| UC-CUS-09 | Track order | Order page shows a timeline. When `OUT_FOR_DELIVERY`, the **delivery code** is shown (also sent by SMS). | |
| UC-CUS-10 | Give code to driver | Tells the driver the code → order `DELIVERED`. | |
| UC-CUS-11 | Report a problem | On a paid order → pick reason + note. Goes to admin. | |
| UC-CUS-12 | Buy again | Shop page shows "Buy again" with past items, **current** price and availability. One tap adds. | Unavailable items shown greyed. |
| UC-CUS-13 | Manage addresses | Add, edit, delete, set default. | |

### A3 Shop owner and A4 Shop staff

| ID | Use case | Main flow | Other flows |
|---|---|---|---|
| UC-SHP-01 | Get new bill alert | Console plays a sound and shows a badge. SMS/WhatsApp alert if enabled. | Console polls every 20 s while open. |
| UC-SHP-02 | Review bill | Opens bill → for each line: OK / change qty / remove / substitute → **Approve**, **Send revision** or **Reject** (with reason). | Approve → `AWAITING_PAYMENT`. Revision → `REVISED`. Reject → `REJECTED`. No action before timeout → `EXPIRED`. |
| UC-SHP-03 | Pick and pack | `PAID` → "Start picking" → pick list sorted by category → "Packed". | Item missing → "Request partial refund" for that line. |
| UC-SHP-04 | Hand over to driver | Assign driver → "Out for delivery". Customer gets the code. | Driver source rules from shop settings. |
| UC-SHP-05 | Request cancel after payment | Reason required. Only before `PACKED`. Admin decides. | |
| UC-SHP-06 | Manage products (manual shops) | Edit price, MRP, stock, available. | Connector-managed fields are **read only** here. |
| UC-SHP-07 | Manage team (owner) | Invite staff or drivers by email, change role, remove. | |
| UC-SHP-08 | See dashboard | Owner: sales, orders, top items, margin deducted, payout due. Staff: today's queue and times. | |

### A5 Driver

| ID | Use case | Main flow | Other flows |
|---|---|---|---|
| UC-DRV-01 | See today's deliveries | List grouped by slot: name, landmark, map link, call button, items count, amount note. | Only orders assigned to this driver. |
| UC-DRV-02 | Complete delivery | Asks customer for the code → enters it → `DELIVERED`. | Wrong code: 5 tries, then locked; admin can unlock. |
| UC-DRV-03 | Mark failed | "Customer not available" + note → `DELIVERY_FAILED`. Shop / admin decides retry. | |
| UC-DRV-04 | See own stats | Deliveries today / this week. | |

### A6 Super admin

| ID | Use case | Main flow |
|---|---|---|
| UC-ADM-01 | Sign in | Google → TOTP code. |
| UC-ADM-02 | Create shop | Name, slug, location, contact, logo, theme color → shop created as `DRAFT`, not accepting orders. |
| UC-ADM-03 | Shop settings | Delivery type, payment mode + account, driver source, notification channels, markup, minimum margin, minimum order, timeouts. |
| UC-ADM-04 | Open hours and switch | Weekly hours. "Accepting orders" on/off. |
| UC-ADM-05 | Slots | Create slots per day (start, end, capacity, cutoff) or copy a week template. |
| UC-ADM-06 | Delivery fee rules | Platform default and per-shop rule. Live preview: "3.2 km, 6 kg, Rs 2,400 bill → Rs 190". |
| UC-ADM-07 | Invite shop owner | Email invite → owner signs in with Google. |
| UC-ADM-08 | Connect shop database | Paste connection string → test → pick table → map columns → preview 20 rows → save → first sync. |
| UC-ADM-09 | Fix sync issues | Unmatched products (link or create master product), held prices (approve / reject). |
| UC-ADM-10 | Master catalog | Products, categories, barcodes, weights, images. |
| UC-ADM-11 | Coupons | Create, pause, see usage. |
| UC-ADM-12 | Cancels and refunds | Approve / reject shop requests and customer problems. Mark refund done with reference. |
| UC-ADM-13 | Payouts | Create payout for a shop and period (amount from ledger), mark paid with bank reference. |
| UC-ADM-14 | Platform dashboard | All shops: GMV, margin, orders, cancel rate, SMS used, failed payments, sync health. |
| UC-ADM-15 | Audit log | Search who changed what. |

### System actors

| ID | Actor | Use case |
|---|---|---|
| UC-SYS-01 | S2 PayHere | Sends payment result to `/v1/webhooks/payhere`. We verify signature, amount, currency, order state. |
| UC-SYS-02 | S6 Scheduler | Fires a timer: review / revision / payment timeout. Timer handler does nothing if the order already moved on. |
| UC-SYS-03 | S5 Shop DB | Read by the `sync` function with a read-only user. |
| UC-SYS-04 | Shop's own admin panel | Calls the signed trigger URL after price or stock change → instant sync. |
| UC-SYS-05 | S1 Cart service | Worker pushes price / availability changes to its price list (batches of 25). |

## 4. What each person must feel

| Actor | Must feel |
|---|---|
| Customer | Fast, clear, honest prices. Never surprised at payment. |
| Shop staff | New bills are impossible to miss. Review is 3 taps. |
| Driver | One screen. Big buttons. Works on a cheap phone. |
| Admin | Everything in one place. Nothing happens without a log. |
