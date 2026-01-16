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

