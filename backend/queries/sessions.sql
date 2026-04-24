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

-- name: UpdateCdmMaster :exec
UPDATE sessions SET cdm_master = $2 WHERE id = $1;

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
WHERE NOT EXISTS (
    SELECT 1
    FROM controllers
    WHERE controllers.session = sessions.id
      AND controllers.last_seen_euroscope > @expired_time
      AND controllers.observer = false
);

-- name: UpdateActiveRunways :exec
UPDATE sessions SET active_runways = $2 WHERE id = $1;

-- name: GetSessions :many
SELECT * FROM sessions;

-- name: GetSessionsByNames :many
SELECT * FROM sessions WHERE name = $1;

-- name: UpdateSessionSids :exec
UPDATE sessions SET available_sids = $2 WHERE id = $1;

-- name: GetSessionSids :one
SELECT available_sids FROM sessions WHERE id = $1;
