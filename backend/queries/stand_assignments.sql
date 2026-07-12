-- name: CreateStandAssignment :one
INSERT INTO stand_assignments (
    session_id, callsign, stand, direction, stage, source, rule_id, tier,
    matched_variant, conflict_reason, eta, eta_source,
    assigned_at, expires_at, manual, acknowledged, acknowledged_at,
    acknowledged_by, vatsim_cid, vatsim_revision
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    $9, $10, $11, $12,
    $13, $14, $15, $16, $17,
    $18, $19, $20
)
RETURNING *;

-- name: GetStandAssignment :one
SELECT *
FROM stand_assignments
WHERE session_id = $1 AND callsign = $2;

-- name: ListStandAssignments :many
SELECT *
FROM stand_assignments
WHERE session_id = $1
ORDER BY callsign;

-- name: LockStandAssignments :many
SELECT *
FROM stand_assignments
WHERE session_id = $1
  AND (callsign = $2 OR expires_at IS NULL OR expires_at > NOW())
ORDER BY callsign
FOR UPDATE;

-- name: UpdateStandAssignment :execrows
UPDATE stand_assignments
SET stand = $3,
    direction = $4,
    stage = $5,
    source = $6,
    rule_id = $7,
    tier = $8,
    matched_variant = $9,
    conflict_reason = $10,
    eta = $11,
    eta_source = $12,
    assigned_at = $13,
    expires_at = $14,
    manual = $15,
    acknowledged = $16,
    acknowledged_at = $17,
    acknowledged_by = $18,
    vatsim_cid = $19,
    vatsim_revision = $20,
    version = version + 1,
    updated_at = NOW()
WHERE id = $1 AND session_id = $2 AND version = $21;

-- name: DeleteStandAssignment :execrows
DELETE FROM stand_assignments
WHERE id = $1 AND session_id = $2 AND version = $3;

-- name: CreateStandBlock :one
INSERT INTO stand_blocks (
    session_id, stand, block_type, source, reason, callsign, created_by,
    expires_at, manual
)
SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9
FROM (SELECT id FROM sessions WHERE id = $1 FOR UPDATE) AS locked_session
RETURNING *;

-- name: GetStandBlock :one
SELECT *
FROM stand_blocks
WHERE id = $1 AND session_id = $2;

-- name: ListStandBlocks :many
SELECT *
FROM stand_blocks
WHERE session_id = $1
ORDER BY stand, id;

-- name: LockActiveManualStandBlocks :many
SELECT *
FROM stand_blocks
WHERE session_id = $1
  AND manual = TRUE
  AND (expires_at IS NULL OR expires_at > NOW())
ORDER BY stand, id
FOR UPDATE;

-- name: ListStandBlocksByStand :many
SELECT *
FROM stand_blocks
WHERE session_id = $1 AND stand = $2
ORDER BY id;

-- name: UpdateStandBlock :execrows
UPDATE stand_blocks
SET stand = $3,
    block_type = $4,
    source = $5,
    reason = $6,
    callsign = $7,
    created_by = $8,
    expires_at = $9,
    manual = $10,
    version = version + 1,
    updated_at = NOW()
WHERE id = $1 AND session_id = $2 AND version = $11;

-- name: DeleteStandBlock :execrows
DELETE FROM stand_blocks
WHERE id = $1 AND session_id = $2 AND version = $3;
