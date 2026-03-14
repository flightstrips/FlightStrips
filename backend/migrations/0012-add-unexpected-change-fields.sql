ALTER TABLE strips ADD COLUMN IF NOT EXISTS unexpected_change_fields TEXT[] DEFAULT '{}';
