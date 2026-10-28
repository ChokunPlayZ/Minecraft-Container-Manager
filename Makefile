.PHONY: dev build test test-deploy lint tidy

BUILD_COMMIT := $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
BUILD_DIRTY := $(shell test -z "$$(git status --porcelain 2>/dev/null)" && echo false || echo true)
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# Build the backend, embedding the frontend build if it exists.
build:
	cd web && pnpm build
	@if [ -d web/dist ]; then \
		mkdir -p internal/web/dist; \
		cp -R web/dist/* internal/web/dist/; \
		echo "embedded web/dist from frontend build"; \
	else \
		echo "web/dist not found; keeping embedded placeholder"; \
	fi
	go build -ldflags "-X github.com/mcm-panel/mcm/internal/buildinfo.Commit=$(BUILD_COMMIT) -X github.com/mcm-panel/mcm/internal/buildinfo.Dirty=$(BUILD_DIRTY) -X github.com/mcm-panel/mcm/internal/buildinfo.BuiltAt=$(BUILD_TIME)" -o mcm ./cmd/mcm

dev:
	go run ./cmd/mcm

test:
	go test ./...

test-deploy:
	PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s deploy/tests -v

lint:
	go vet ./...
	@out=$$(gofmt -l .); \
	if [ -n "$$out" ]; then \
		echo "gofmt needed on:"; \
		echo "$$out"; \
		exit 1; \
	fi

tidy:
	go mod tidy

.PHONY: test-web verify test-race
test-web:
	cd web && pnpm typecheck && pnpm lint && pnpm test && pnpm build

test-race:
	go test -race ./internal/servers ./internal/api ./internal/backups ./internal/proxy ./internal/auth ./internal/storage

# The release gate covers backend, browser contracts and deployment transactions.
verify: build
	$(MAKE) lint test test-race test-deploy test-web
