-- name: CreateTacticalStrip :one
INSERT INTO tactical_strips (session_id, type, bay, label, aircraft, produced_by, sequence)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListTacticalStripsByBay :many
SELECT * FROM tactical_strips
WHERE session_id = $1 AND bay = $2
ORDER BY sequence ASC;

-- name: ListTacticalStripsBySession :many
SELECT * FROM tactical_strips
WHERE session_id = $1
ORDER BY bay, sequence ASC;

-- name: DeleteTacticalStrip :exec
DELETE FROM tactical_strips WHERE id = $1 AND session_id = $2;

-- name: ConfirmTacticalStrip :one
UPDATE tactical_strips
SET confirmed = TRUE, confirmed_by = $3
WHERE id = $1 AND session_id = $2
RETURNING *;

-- name: StartTacticalStripTimer :one
UPDATE tactical_strips
SET timer_start = NOW()
WHERE id = $1 AND session_id = $2
RETURNING *;

-- name: UpdateTacticalStripSequence :one
UPDATE tactical_strips
SET sequence = sqlc.arg(sequence)::INT
WHERE id = $1 AND session_id = $2
RETURNING *;

-- name: GetTacticalStripSequenceByID :one
SELECT sequence::INT FROM tactical_strips WHERE id = $1 AND session_id = $2;

-- name: ListTacticalStripBaySequences :many
SELECT id, sequence FROM tactical_strips
WHERE session_id = $1 AND bay = sqlc.arg(bay)::TEXT
ORDER BY sequence, id;
