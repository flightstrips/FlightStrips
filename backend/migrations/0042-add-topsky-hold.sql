-- TopSky holding clearances, read off the EuroScope scratch pad by the plugin.
-- Empty means not holding, which is the overwhelmingly common case, so these are
-- NOT NULL with an empty default rather than nullable.
ALTER TABLE strips ADD COLUMN IF NOT EXISTS hold varchar NOT NULL DEFAULT '';
ALTER TABLE strips ADD COLUMN IF NOT EXISTS hold_type varchar NOT NULL DEFAULT '';
ALTER TABLE strips ADD COLUMN IF NOT EXISTS hold_eat varchar NOT NULL DEFAULT '';
