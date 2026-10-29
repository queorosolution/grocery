# Run from the repo root. Generated files are committed, so CI runs `make gen` and checks for no diff.
.PHONY: gen check-contracts test test-schema migrate-local lint

gen:
	cd backend && go run ./cmd/gen -contracts ../contracts -backend . -web ../web

check-contracts:
	cd backend && go run ./cmd/gen -check -contracts ../contracts

test: check-contracts
	cd backend && go test ./...
	@if [ -f web/package.json ]; then cd web && npm test --if-present; fi

# Applies the migrations to a fresh database and runs contracts/schema_test.sql (needs psql).
test-schema:
	./scripts/test-schema.sh

# Owner URL for a local Postgres (docs/14 section 6). Create the database first: createdb gp
migrate-local:
	cd backend && MIGRATE_DATABASE_URL="$${MIGRATE_DATABASE_URL:-postgresql://postgres:postgres@localhost:5432/gp?sslmode=disable}" go run ./cmd/migrate up

lint:
	@test -z "$$(gofmt -l backend)" || (gofmt -l backend; echo "gofmt: run gofmt -w"; exit 1)
	cd backend && go vet ./...
	@if [ -f web/package.json ]; then cd web && npm run lint --if-present; fi
