FROM golang:1.26.1-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY internal/ ./internal/
COPY cmd/ ./cmd/
RUN CGO_ENABLED=0 GOOS=linux go build -o memex ./cmd/memex

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/memex /usr/local/bin/memex
EXPOSE 8765
CMD ["memex", "serve"]
