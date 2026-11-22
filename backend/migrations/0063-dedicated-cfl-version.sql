-- A repeated dedicated CFL write is still authoritative over older full syncs.
-- Keep its strip version separately from unrelated edits to the same row.
ALTER TABLE strips ADD COLUMN dedicated_cfl_version INTEGER NOT NULL DEFAULT 0;
