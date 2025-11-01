-- name: CreateCoordination :one
INSERT INTO coordinations (session, strip_id, from_position, to_position)
VALUES ($1, $2, $3, $4) RETURNING *;

-- name: ListCoordinationsBySession :many
SELECT *
FROM coordinations
WHERE session = $1
ORDER BY coordinated_at DESC;

-- name: DeleteCoordinationByID :execrows
DELETE
FROM coordinations
WHERE id = $1;

-- name: GetCoordinationByStripID :one
SELECT *
FROM coordinations
WHERE session = $1 AND strip_id = $2;

-- name: GetCoordinationByStripCallsign :one
SELECT c.*
FROM coordinations c
         JOIN strips s ON s.id = c.strip_id
WHERE s.session = $1
  AND s.callsign = $2;
