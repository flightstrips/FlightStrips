ALTER TABLE strips
    ADD COLUMN IF NOT EXISTS pdc_data jsonb NOT NULL DEFAULT '{}'::jsonb;

UPDATE strips
SET pdc_data = jsonb_strip_nulls(
    jsonb_build_object(
        'state', CASE
            WHEN pdc_state IS NOT NULL AND pdc_state <> 'NONE' THEN pdc_state
            ELSE NULL
        END,
        'requestRemarks', pdc_request_remarks,
        'requestedAt', pdc_requested_at,
        'messageSequence', pdc_message_sequence,
        'messageSent', pdc_message_sent
    )
);

ALTER TABLE strips
    DROP COLUMN IF EXISTS pdc_state,
    DROP COLUMN IF EXISTS pdc_requested_at,
    DROP COLUMN IF EXISTS pdc_message_sequence,
    DROP COLUMN IF EXISTS pdc_message_sent,
    DROP COLUMN IF EXISTS pdc_request_remarks;
