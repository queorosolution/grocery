# Web (Next.js static export)

Design: `docs/08-frontend-design.md`. Config: `docs/13-config.md` section 3.

```bash
cp .env.example .env.local   # public values only
npm ci
npm run dev                  # http://localhost:3000 (needs `make dev-api` for data)
npm test                     # unit tests (Vitest)
npm run lint && npm run typecheck
npm run build                # static files in out/
```

## Rules that bite

- `npm run build` = shop list, rewrites, `next build`, CSP hashes. It never fails because the API is down: the shop list is then empty.
- Add a page with an id in its path (`/order/[id]`)? Run `npm run gen:redirects` and commit `public/_redirects`.
- Read ids with `useRouteId()`, never `params` (ESLint stops you).
- Only the `NEXT_PUBLIC_` names in `docs/13` section 3 may exist (`npm run check:env`).
- Phone first: build for 360 px, then widen with `md:` and `lg:`.
