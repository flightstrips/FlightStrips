ALTER TABLE strips
    ADD COLUMN IF NOT EXISTS cdm_data jsonb NOT NULL DEFAULT '{}'::jsonb;

UPDATE strips
SET cdm_data = jsonb_strip_nulls(
    jsonb_build_object(
        'canonical', jsonb_build_object(
            'tobt', tobt,
            'tsat', tsat,
            'ttot', ttot,
            'ctot', ctot,
            'aobt', aobt,
            'asat', asat,
            'eobt', eobt,
            'status', cdm_status
        )
    )
);

ALTER TABLE strips
    DROP COLUMN IF EXISTS tobt,
    DROP COLUMN IF EXISTS tsat,
    DROP COLUMN IF EXISTS ttot,
    DROP COLUMN IF EXISTS ctot,
    DROP COLUMN IF EXISTS aobt,
    DROP COLUMN IF EXISTS asat,
    DROP COLUMN IF EXISTS eobt,
    DROP COLUMN IF EXISTS cdm_status;
