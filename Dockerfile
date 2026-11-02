# syntax=docker/dockerfile:1

# ---- Build stage ------------------------------------------------------------
FROM golang:1.23-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

# ---- Runtime stage ----------------------------------------------------------
# distroless/static: CA certs + tzdata only; no shell, no package manager, non-root.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /app/api
COPY --from=build /out/migrate /app/migrate

USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/api"]
