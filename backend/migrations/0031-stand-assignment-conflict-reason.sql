ALTER TABLE stand_assignments
    ADD COLUMN IF NOT EXISTS conflict_reason VARCHAR;
