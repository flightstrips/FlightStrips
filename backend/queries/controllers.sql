-- name: InsertController :exec
INSERT INTO controllers (callsign, session, position, cid, last_seen_euroscope, last_seen_frontend)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: BulkInsertControllers :copyfrom
INSERT INTO controllers (callsign, session, position)
VALUES ($1, $2, $3);

-- name: SetControllerPosition :execrows
UPDATE controllers
SET position = $1
WHERE callsign = $2 AND session = $3;

-- name: SetControllerEuroscopeSeen :execrows
UPDATE controllers
SET last_seen_euroscope = $1
WHERE cid = @cid::text AND session = $2;

-- name: SetControllerFrontendSeen :execrows
UPDATE controllers
SET last_seen_frontend = $1
WHERE cid = @cid::text AND session = $2;

-- name: SetControllerCid :execrows
UPDATE controllers
SET cid = $1
WHERE callsign = $2 AND session = $3;

-- name: GetControllerByCid :one
SELECT *
FROM controllers
WHERE cid = @cid::text LIMIT 1;

-- name: ListControllers :many
SELECT *
FROM controllers
WHERE session = $1
ORDER BY callsign;

-- name: RemoveController :execrows
DELETE
FROM controllers
WHERE callsign = $1 AND session = $2;

-- name: GetController :one
SELECT *
FROM controllers
WHERE callsign = $1 AND session = $2;

-- name: GetControllers :many
SELECT * FROM controllers
WHERE session = $1;

-- name: SetControllerLayout :execrows
UPDATE controllers
SET layout = $1
WHERE position = $2 AND session = $3;