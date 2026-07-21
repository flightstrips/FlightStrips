ALTER TABLE tactical_strips
    ADD COLUMN owner TEXT,
    ADD COLUMN marked BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE tactical_strips
SET owner = produced_by
WHERE owner IS NULL;

ALTER TABLE tactical_strips
    ALTER COLUMN owner SET NOT NULL,
    DROP COLUMN timer_start;
