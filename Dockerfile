# syntax=docker/dockerfile:1

# Build stage
FROM --platform=$BUILDPLATFORM golang:1.26.4-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=none
ARG BUILD_DATE=unknown

WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build a static, CGO-free binary. Same ldflags var paths as the Makefile
# (main.Version/main.Commit/main.BuildDate) so `hostus version` reports real
# build info both locally and inside the image.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOFLAGS=-trimpath \
    go build -ldflags "-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildDate=${BUILD_DATE}" \
    -o hostus ./cmd/hostus

# Final stage - distroless static, non-root
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /build/hostus /hostus

# Numeric UID:GID (the distroless "nonroot" user, 65532:65532) rather than the
# name, so it resolves without relying on the image's /etc/passwd entry.
USER 65532:65532

EXPOSE 8080

# No HEALTHCHECK here: gcr.io/distroless/static has no shell and no HTTP
# client (no curl/wget), so neither shell-form nor CMD-form health probes
# can run inside this image. Liveness is checked externally by the
# orchestrator (Docker/Kubernetes) against GET /health/live, which the
# service exposes on its configured port. A future `hostus health` self-probe
# subcommand is a possible option but is out of scope here (untested Go
# logic outside this task).
ENTRYPOINT ["/hostus"]
