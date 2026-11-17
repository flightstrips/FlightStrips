# Build stage
FROM golang:1.27-alpine AS builder
WORKDIR /app

RUN apk add --no-cache gcc musl-dev

COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG BUILD_VERSION
RUN go build -ldflags "-w -s -X main.buildVersion=${BUILD_VERSION}" -a -o ./bin/api ./cmd/server

# Final stage
FROM alpine:latest
WORKDIR /app/backend

RUN apk --no-cache add ca-certificates

COPY --from=builder /app/bin/api /app/bin/api
COPY ./config /app/backend/config

ENV AIRPORT_REPOSITORY_ROOT=/app
ENV AIRPORT_CONFIG_FILES=backend/config/airports/ekch.airport.yaml

CMD ["/app/bin/api"]
EXPOSE 8090
