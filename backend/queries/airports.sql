-- name: InsertAirport :exec
INSERT INTO airports (name)
VALUES ($1);
