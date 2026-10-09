-- Shared controller text is owned by FS, never imported from ES snapshots.
ALTER TABLE strips ADD COLUMN fs_scratch_pad TEXT NOT NULL DEFAULT '';
