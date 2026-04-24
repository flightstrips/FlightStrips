ALTER TABLE controllers
    ADD COLUMN IF NOT EXISTS observer boolean NOT NULL DEFAULT false;
