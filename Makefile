# Run from the repo root. Generated files are committed, so CI runs `make gen` and checks for no diff.
.PHONY: gen check-contracts test test-schema migrate-local lint lint-go dev-api validate build migrate deploy

export SAM_CLI_TELEMETRY := 0

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

# Go only. The backend CI job has no node_modules, so it runs this; the web job runs the web lint itself.
lint-go:
	@test -z "$$(gofmt -l backend)" || (gofmt -l backend; echo "gofmt: run gofmt -w"; exit 1)
	cd backend && go vet ./...

lint: lint-go
	@if [ -f web/package.json ]; then cd web && npm run lint --if-present; fi

# Runs the API on :8080 (needs a .env copied from .env.example; refuses to run unless APP_ENV=local).
dev-api:
	cd backend && ENV_FILE=../.env go run ./cmd/devserver

# --- SAM (needs the AWS SAM CLI) ---

# Checks backend/template.yaml and lints it.
validate:
	cd backend && sam validate --lint

# Builds the four Lambda functions into backend/.aws-sam.
build:
	cd backend && sam build

# Applies migrations with the OWNER url from your shell: MIGRATE_DATABASE_URL=... make migrate
migrate:
	cd backend && go run ./cmd/migrate up

# Deploys one environment. SAM_PARAMETERS holds the parameter overrides (docs/14 section 5).
# Every deploy needs the owner to say yes; CI does this on merge to main and on a v* tag.
deploy:
	@test -n "$(ENV)" || (echo "usage: make deploy ENV=dev|prod"; exit 1)
	@test -n "$$SAM_PARAMETERS" || (echo "set SAM_PARAMETERS (see docs/14-deployment.md section 5)"; exit 1)
	@test -n "$(NO_BUILD)" || $(MAKE) build
	cd backend && sam deploy --config-env $(ENV) --parameter-overrides "$$SAM_PARAMETERS"
