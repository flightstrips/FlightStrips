ALTER TABLE strips
    ADD COLUMN IF NOT EXISTS vatsim_cid VARCHAR,
    ADD COLUMN IF NOT EXISTS vatsim_revision BIGINT,
    ADD COLUMN IF NOT EXISTS vatsim_seen_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS euroscope_seen_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_strips_vatsim_source
    ON strips (session, vatsim_seen_at)
    WHERE vatsim_seen_at IS NOT NULL;
