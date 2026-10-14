# FlightStrips API

The FlightStrips API is written in Go.

## Arrival stand lifecycle

Automatic arrival stand assignments progress through these stages:

- **ESTIMATED:** Planning normally starts within 45 minutes of arrival ETA.
- **ASSIGNED:** The aircraft is within 25 NM of the destination airport's
  configured stands and at or below 10,000 ft. ETA does not trigger this stage.
- **CONFIRMED:** Arrival ETA is within 2 minutes and altitude is at or below
  3,000 ft, regardless of distance. Detection within the airport area (2 NM
  of its configured stands and at or below 1,000 ft) also confirms immediately.

Stages do not downgrade. Confirmed stands are protected from subsequent
automatic planning changes; controller actions and physical-position
reconciliation remain supported.

Departure stand availability is projected at TSAT +5 minutes, falling back to
TOBT +5 minutes when TSAT is missing. Once startup is requested (START REQ or
START REQ+TRF), the projection becomes the later of the recorded actual startup
request time (ASRT) and TSAT -5 minutes, falling back to TOBT -5 minutes.
An assigned PUSH bay or ground state removes the normal buffer. PUSH or observed
movement away from the stands releases capacity immediately; a missed
projection does not release a still-occupied stand.

## Starting the application

The backend application can run in docker using: 

```sh
docker compose --profile all up --build -d
```

For local development only the database can be started using:

```sh
docker compose --profile database up --build -d
```

Run the complete backend test suite with one shared PostgreSQL server and an
isolated database for every database-backed test:

```sh
go run ./internal/testing/testdb ./...
```

Arguments after `testdb` are passed to `go test`, so package and `-run` filters
continue to work. Direct `go test` package runs remain supported and start a
package-local fallback server when needed.

When running the backend locally with `go run`, `.env.dev` is loaded after `.env` for development-only overrides. Aspire OTLP export is configured there and is not included in the Docker image.
