.PHONY: setup dev dev-api dev-web build generate generate-check test lint check clean brand

GO ?= go
PNPM ?= pnpm
VERSION ?= 0.1.0-dev
WEB_PORT ?= 5173
SQLC = $(GO) run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1

setup:
	$(GO) mod download
	$(PNPM) --dir web install --frozen-lockfile

dev:
	node scripts/dev.mjs --port=$(WEB_PORT)

dev-api:
	$(GO) run ./cmd/sublane

dev-web:
	$(PNPM) --dir web dev

build:
	$(PNPM) --dir web build
	CGO_ENABLED=0 $(GO) build -tags production -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o bin/sublane ./cmd/sublane

generate:
	$(SQLC) generate

generate-check:
	$(SQLC) diff

test:
	$(GO) test -race ./...
	$(PNPM) --dir web test

lint:
	$(GO) vet ./...
	@test -z "$$(gofmt -l cmd internal web/*.go)" || (echo 'Run gofmt before checking.'; exit 1)
	$(PNPM) --dir web typecheck
	$(PNPM) --dir web lint
	$(PNPM) --dir web format:check

check: generate-check lint test build

clean:
	rm -rf bin web/dist dist

brand:
	$(PNPM) --dir tools/brand install --frozen-lockfile
	$(PNPM) --dir tools/brand build
	$(PNPM) --dir tools/brand check
