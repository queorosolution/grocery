# CLAUDE.md — rules for building this project

You are building a multi-shop grocery platform. The specs in `docs/` and `contracts/` are the source of truth. Work in **small steps** from `PROMPTS.md`.

## 1. Before you write code

1. Read **only** the files the prompt lists. Do not read the whole `docs/` folder.
2. If the prompt and a doc disagree, or something is missing, **stop and ask**. Do not guess. Do not invent endpoints, fields, statuses or tables.
3. Check whether a helper already exists (`docs/07-backend-design.md` §2, `docs/08-frontend-design.md` §3–5). Use it. Never write a second version.

## 2. Hard rules (never break)

- **Contracts first.** Roles, statuses, permissions, transitions and error codes come from `contracts/enums.json` via `make gen`. Never type these strings by hand. Never edit `*_gen.go` / `*.gen.ts`.
- **Money:** `int64` cents in Go, `number` of cents in TS. No floats for money. Only `billing.Build` adds up an order.
- **Prices are server-side.** Never accept a price, total, status, role or `customerId` from a request body.
- **Orders change status only through `order.Transition`.** No other `UPDATE orders SET status`.
- **Every transaction goes through `db.Tx(ctx, scope, fn)`.** No raw `pool.Begin`.
- **SQL uses parameters.** No `fmt.Sprintf` to build SQL. Identifiers only through the connector allow-list + `pgx.Identifier`.
- **Secrets:** never in code, env values, logs or API responses. Read them with `secrets.Get`.
- **Frontend:** no `dangerouslySetInnerHTML`, no tokens in `localStorage`, no `fetch` outside `lib/api.ts` / `lib/cartApi.ts`, no `fetch` in `useEffect`, buttons from `allowedActions` only.
- **Next.js is a static export** (`docs/08-frontend-design.md` §11): screens are `"use client"`; never use Server Actions, Route Handlers (`app/api`), `middleware.ts`, `cookies()`, `headers()`, ISR or `next/image` optimisation. Ids in id pages come from `useRouteId()`, never `params`. Detail routes use singular paths (`/order/[id]`). Re-run `npm run gen:redirects` after adding an id route. Only the `NEXT_PUBLIC_` names in `docs/13-config.md` §3 may exist.
- **Dependencies:** only the lists in `07` §15 and `08` §1. Need another? Stop and ask.
- **Brand:** never hard-code the platform name or domain. Use config.

## 3. Code shape

- Backend layers: `handler.go` → `service.go` → `repo.go`. Handlers have no SQL; repos have no rules.
- A module calls another module's **service**, never its repo.
- New route = one line in `internal/http/routes.go` with its guard. A route without a guard fails the build test.
- Frontend layers: `app/**/page.tsx` (thin, renders one screen) → `features/<x>/` screen (`"use client"`) → `features/<x>/api.ts` hooks → `lib/api.ts`. Shared visuals only in `components/ui/`.
- Max file length: Go 300 lines, TS/TSX 200 lines. Split by responsibility.
- Names and folders as in `docs/12-file-structure.md`.
- Errors: return `apperr` codes from the contract. Messages to users come from `error_codes`.
- Comments explain **why**, not what.

## 4. Keep the flow working

- Do not change code outside the prompt's scope. No drive-by refactors.
- Do not rename or move existing files unless the prompt says so.
- If a change touches an API shape, update `docs/06-api.md` in the same step and tell the user.
- If a test you did not write fails, stop and report it — do not "fix" it by changing the test.

## 5. Definition of done (every prompt)

- [ ] `make gen` (if contracts changed), `make test`, `make lint` pass.
- [ ] New code has tests. The prompt's listed tests and security IDs pass.
- [ ] No new dependency outside the allowed lists.
- [ ] No duplicated helper, no file over the limit.
- [ ] Short summary to the user: what changed, files touched, anything left open.

## 6. Commits

`<area>: <what>` + requirement / check IDs. Example: `order: submit bill with slot booking (FR-BILL-02, BIZ-04)`.
