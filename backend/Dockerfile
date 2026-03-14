# Build stage
FROM golang:1.25-alpine AS builder
WORKDIR /app

RUN apk add --no-cache gcc musl-dev

COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN go build -ldflags '-w -s' -a -o ./bin/api ./cmd/server

# Final stage
FROM alpine:latest
WORKDIR /app

RUN apk --no-cache add ca-certificates

COPY --from=builder /app/bin/api ./bin/api
COPY ./config ./config
COPY .env .env

CMD ["/app/bin/api"]
EXPOSE 8090
