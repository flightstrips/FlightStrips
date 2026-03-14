ALTER TABLE strips ADD COLUMN IF NOT EXISTS controller_modified_fields TEXT[] DEFAULT '{}';
