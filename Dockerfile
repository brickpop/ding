# ── Build stage ────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /ding .

# ── Runtime stage (distroless, ~4MB) ──────────────────────────
FROM gcr.io/distroless/static

COPY --from=builder /ding /ding
ENTRYPOINT ["/ding"]
