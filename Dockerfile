# --- Build ---
FROM golang:1.26.9-alpine AS builder
RUN apk add --no-cache git ca-certificates tzdata
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" \
    -o /out/chat-service ./cmd/server

# --- Run ---
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /out/chat-service /app/chat-service

WORKDIR /app
USER nonroot:nonroot
EXPOSE 4005

ENTRYPOINT ["/app/chat-service"]
