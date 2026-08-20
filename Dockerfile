# syntax=docker/dockerfile:1

###############################################################################
# Web build stage
###############################################################################
FROM node:22-alpine AS web

WORKDIR /app

COPY web/ ./web/
WORKDIR /app/web

# pnpm is not present in the base image; install it to match the lockfile.
RUN npm install -g pnpm@11

# Use the lockfile when available, otherwise fall back to a fresh install.
RUN if [ -f pnpm-lock.yaml ]; then \
        pnpm install --frozen-lockfile; \
    else \
        pnpm install; \
    fi

RUN pnpm build

###############################################################################
# Go build stage
###############################################################################
FROM golang:1.26-alpine AS go

WORKDIR /src

# Copy the module manifest first, then the source. go.sum is required so the
# build can verify module checksums.
COPY go.mod go.sum ./

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY migrations/ ./migrations/

# Overwrite the committed placeholder with the web worker's real build output.
COPY --from=web /app/web/dist ./internal/web/dist

RUN go mod download && \
    CGO_ENABLED=0 go build -o /out/mcm ./cmd/mcm

###############################################################################
# Runtime stage
###############################################################################
FROM alpine:3.20 AS runtime

RUN apk add --no-cache ca-certificates tzdata su-exec

COPY --from=go /out/mcm /usr/local/bin/mcm
COPY docker/entrypoint.sh /entrypoint.sh

RUN chmod +x /entrypoint.sh && mkdir -p /data

ENV MCM_ADDR=:8080 \
    MCM_DATA_DIR=/data

EXPOSE 8080

ENTRYPOINT ["/entrypoint.sh"]
