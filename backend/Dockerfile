# Build stage
FROM golang:1.25-alpine AS builder
WORKDIR /app

RUN apk add --no-cache gcc musl-dev

COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG BUILD_VERSION=dev
RUN go build -ldflags "-w -s -X main.buildVersion=${BUILD_VERSION}" -a -o ./bin/api ./cmd/server
RUN go build -ldflags "-w -s" -o ./bin/nats-bootstrap ./cmd/nats-bootstrap

# Final stage
FROM alpine:latest
WORKDIR /app

RUN apk --no-cache add ca-certificates

COPY --from=builder /app/bin/api ./bin/api
COPY --from=builder /app/bin/nats-bootstrap ./bin/nats-bootstrap
COPY ./config ./config

CMD ["/app/bin/api"]
EXPOSE 8090
