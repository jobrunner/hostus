# syntax=docker/dockerfile:1

# Build stage
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w -X main.version=$(cat VERSION)" \
    -o hostus ./cmd/hostus

# Final stage - distroless
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /build/hostus /hostus
COPY --from=builder /build/VERSION /VERSION

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/hostus"]
