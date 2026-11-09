# build
FROM golang:1.27-alpine AS builder
WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/api ./cmd/api \
 && CGO_ENABLED=0 GOOS=linux go build -o /bin/migrate ./cmd/migrate \
 && CGO_ENABLED=0 GOOS=linux go build -o /bin/seed ./cmd/seed

# run
FROM alpine:3.20
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 1000 appuser

COPY --from=builder /bin/api /bin/migrate /bin/seed /usr/local/bin/
COPY migrations ./migrations

RUN mkdir -p /app/storage/exports \
 && chown -R appuser:appuser /app

USER appuser
EXPOSE 8080

CMD ["api"]