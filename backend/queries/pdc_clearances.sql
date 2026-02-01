-- name: GetNextPdcSequence :one
UPDATE sessions SET pdc_sequence = pdc_sequence + 1 WHERE id = $1 RETURNING pdc_sequence;

-- name: GetNextMessageSequence :one
UPDATE sessions SET pdc_message_sequence = pdc_message_sequence + 1 WHERE id = $1 RETURNING pdc_message_sequence;

-- name: SetPdcRequested :exec
UPDATE strips SET pdc_state = $3, pdc_requested_at = $4 WHERE callsign = $1 AND session = $2;

-- name: UpdatePdcStatus :exec
UPDATE strips SET pdc_state = $3 WHERE callsign = $1 AND session = $2;

-- name: SetPdcMessageSent :exec
UPDATE strips SET pdc_state = $3, pdc_message_sequence = $4, pdc_message_sent = $5 WHERE callsign = $1 AND session = $2;
