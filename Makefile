# Run from the repo root. Generated files are committed, so CI runs `make gen` and checks for no diff.
.PHONY: gen check-contracts test lint

gen:
	cd backend && go run ./cmd/gen -contracts ../contracts -backend . -web ../web

check-contracts:
	cd backend && go run ./cmd/gen -check -contracts ../contracts

test: check-contracts
	cd backend && go test ./...
	@if [ -f web/package.json ]; then cd web && npm test --if-present; fi

lint:
	@test -z "$$(gofmt -l backend)" || (gofmt -l backend; echo "gofmt: run gofmt -w"; exit 1)
	cd backend && go vet ./...
	@if [ -f web/package.json ]; then cd web && npm run lint --if-present; fi
