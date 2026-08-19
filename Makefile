.PHONY: dev build test lint tidy

# Build the backend, embedding the frontend build if it exists.
build:
	cd web && pnpm build
	@if [ -d web/dist ]; then \
		cp -R web/dist/* internal/web/dist/; \
		echo "embedded web/dist from frontend build"; \
	else \
		echo "web/dist not found; keeping embedded placeholder"; \
	fi
	go build -o mcm ./cmd/mcm

dev:
	go run ./cmd/mcm

test:
	go test ./...

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
