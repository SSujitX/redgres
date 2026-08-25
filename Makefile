# Mirrors .github/workflows/ci.yml. The workflow is authoritative; update both in the same change.
.PHONY: fmt vet test test-race frontend-test frontend-build installer integration-test playwright-test build verify

fmt:
	gofmt -w cmd internal migrations integration

vet:
	go vet ./...

test:
	go test ./...

test-race:
	go test -race ./...

frontend-test:
	cd web && npm ci && npm run test:run

frontend-build:
	cd web && npm ci && npm run build

installer:
	bash deploy/tests/run.sh

integration-test:
	go test -count=1 ./integration

playwright-test:
	cd web && npm ci && npx playwright install --with-deps chromium && npm run test:e2e

vet:
	go vet ./...

test:
	go test ./...

test-race:
	go test -race ./...

frontend-test:
	cd web && npm ci && npm run test:run

frontend-build:
	cd web && npm ci && npm run build

build: frontend-build
	go build -o redgres ./cmd/redgres

verify: vet test frontend-test frontend-build
	go build -o redgres ./cmd/redgres
