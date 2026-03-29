-- name: InsertPdcWebRequest :one
INSERT INTO pdc_web_requests (
    session_id, callsign, vatsim_cid, atis, stand, remarks, status, expires_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING id, session_id, callsign, vatsim_cid, atis, stand, remarks, status, clearance_text, error_message, created_at, expires_at, pilot_acknowledged_at;

-- name: GetPdcWebRequestByID :one
SELECT id, session_id, callsign, vatsim_cid, atis, stand, remarks, status, clearance_text, error_message, created_at, expires_at, pilot_acknowledged_at
FROM pdc_web_requests WHERE id = $1;

-- name: GetPendingPdcWebRequestBySessionCallsign :one
SELECT id, session_id, callsign, vatsim_cid, atis, stand, remarks, status, clearance_text, error_message, created_at, expires_at, pilot_acknowledged_at
FROM pdc_web_requests
WHERE session_id = $1 AND callsign = $2 AND status = 'pending'
ORDER BY created_at DESC
LIMIT 1;

-- Pilot still waiting for web clearance: submitted, or faults while ATC fixes the plan.
-- name: GetAwaitingWebPdcBySessionCallsign :one
SELECT id, session_id, callsign, vatsim_cid, atis, stand, remarks, status, clearance_text, error_message, created_at, expires_at, pilot_acknowledged_at
FROM pdc_web_requests
WHERE session_id = $1 AND lower(callsign) = lower($2) AND status IN ('pending', 'faults')
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdatePdcWebRequestStatus :exec
UPDATE pdc_web_requests SET status = $2, error_message = $3 WHERE id = $1;

-- name: UpdatePdcWebRequestClearance :exec
UPDATE pdc_web_requests SET status = $2, clearance_text = $3 WHERE id = $1;

-- name: UpdatePdcWebRequestPilotAck :exec
UPDATE pdc_web_requests SET pilot_acknowledged_at = $2 WHERE id = $1 AND vatsim_cid = $3 AND status = 'cleared';
