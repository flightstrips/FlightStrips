ALTER TABLE pdc_web_requests
    ADD COLUMN IF NOT EXISTS pilot_acknowledged_at TIMESTAMPTZ;
