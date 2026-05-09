# FlightStrips API

The FlightStrips API is written in Go.

## Starting the application

The backend application can run in docker using: 

```sh
docker compose --profile all up --build -d
```

For local development only the database can be started using:

```sh
docker compose --profile database up --build -d
```

When running the backend locally with `go run`, `.env.dev` is loaded after `.env` for development-only overrides. Aspire OTLP export is configured there and is not included in the Docker image.

