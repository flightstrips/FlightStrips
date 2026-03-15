ALTER TABLE sessions ADD COLUMN available_sids jsonb NOT NULL DEFAULT '[]';
