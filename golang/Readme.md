# Golang based backend attempt for Flightstrips

Websocket library: https://github.com/gorilla/websocket

## Starting the application

The backend application can run in docker using: 

```sh
docker compose --profile all up --build -d
```

For local development only the database can be started using:

```sh
docker compose --profile database up --build -d
```

## TODO:

https://studio.asyncapi.com/
take yaml document contents to above URL

Initialise WebSockets

Define Events


## TODO:



## Client Testing
Create a client that can simulate the Front End.
Create a client that can simulate the Euroscope plugin.
Run up to 15 connections of both simultaneously whilst delivering similar traffic.

