# Backend

The backend in written in Go and uses the standard library as much as possible. It exposes an API over websockets and connects to a PostgreSQL database.

The following instructions apply to all files in this directory and its subdirectories.

## Clients

There are two main clients which connects to backend: the web client and the euroscope client. These each have different events and data structures. Make sure to check which client the code is targeting.

An interaction between the clients may sometimes be needed. The Euroscope client is the main provider of data and the web client is mainly used for visualization and user interaction. A web client is only allowed to interact if the user also has an active Euroscope client connection. 

IMPORTANT any changes the web client makes that needs to be reflected in the Euroscope client must be communicated to the Euroscope client of the same user.
