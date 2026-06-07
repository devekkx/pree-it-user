### Build stage ###
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /build/user-service ./cmd/user


### Production stage ###
FROM alpine:3.22.4 AS production

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S appgroup \
    && adduser -S appuser -G appgroup

COPY --from=builder /build/user-service /usr/local/bin/user-service

USER appuser

EXPOSE 8083

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8083/health || exit 1

ENTRYPOINT ["user-service"]


### Development stage ###
FROM golang:1.26-alpine AS dev

RUN apk add --no-cache git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go install github.com/air-verse/air@latest

EXPOSE 8083

CMD ["air", "-c", ".air.toml"]