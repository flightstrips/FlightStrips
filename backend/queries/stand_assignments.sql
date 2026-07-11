-- name: CreateStandAssignment :one
INSERT INTO stand_assignments (
    session_id, callsign, stand, direction, stage, source, rule_id, tier,
    matched_variant, eta, eta_source,
    assigned_at, expires_at, manual, acknowledged, acknowledged_at,
    acknowledged_by, vatsim_cid, vatsim_revision
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8,
    $9, $10, $11,
    $12, $13, $14, $15, $16,
    $17, $18, $19
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

-- name: UpdateStandAssignment :execrows
UPDATE stand_assignments
SET stand = $3,
    direction = $4,
    stage = $5,
    source = $6,
    rule_id = $7,
    tier = $8,
    matched_variant = $9,
    eta = $10,
    eta_source = $11,
    assigned_at = $12,
    expires_at = $13,
    manual = $14,
    acknowledged = $15,
    acknowledged_at = $16,
    acknowledged_by = $17,
    vatsim_cid = $18,
    vatsim_revision = $19,
    version = version + 1,
    updated_at = NOW()
WHERE id = $1 AND session_id = $2 AND version = $20;

-- name: DeleteStandAssignment :execrows
DELETE FROM stand_assignments
WHERE id = $1 AND session_id = $2 AND version = $3;

-- name: CreateStandBlock :one
INSERT INTO stand_blocks (
    session_id, stand, block_type, source, reason, callsign, created_by,
    expires_at, manual
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
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
