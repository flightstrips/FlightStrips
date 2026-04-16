-- name: GetNextPdcSequence :one
UPDATE sessions SET pdc_sequence = pdc_sequence + 1 WHERE id = $1 RETURNING pdc_sequence;

-- name: GetNextMessageSequence :one
UPDATE sessions SET pdc_message_sequence = pdc_message_sequence + 1 WHERE id = $1 RETURNING pdc_message_sequence;
