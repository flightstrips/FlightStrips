-- name: InsertController :exec
INSERT INTO controllers (
    cid, airport, position
) VALUES (
             ?, ?, ?
         );

-- name: ListControllers :many
SELECT * FROM controllers;

-- name: ListControllersByAirport :many
SELECT * FROM controllers WHERE airport = ?;

-- name: RemoveController :exec
DELETE FROM controllers WHERE cid = ?;