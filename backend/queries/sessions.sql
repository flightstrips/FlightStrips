-- name: ListSessionsByAirport :many
SELECT *
FROM sessions
WHERE airport = $1;

-- name: DeleteSession :execrows
DELETE
FROM sessions
WHERE id = $1;

-- name: InsertSession :one
INSERT INTO sessions (name, airport)
VALUES ($1, $2) RETURNING id;

-- name: GetSession :one
SELECT *
FROM sessions
WHERE airport = $1
  AND name = $2;

-- name: GetSessionById :one
SELECT *
FROM sessions
WHERE id = $1;

-- name: GetExpiredSessions :many
SELECT id
FROM sessions
WHERE NOT EXISTS (SELECT 1 FROM controllers WHERE last_seen_euroscope > @expired_time);

-- name: UpdateActiveRunways :exec
UPDATE sessions SET active_runways = $2 WHERE id = $1;
