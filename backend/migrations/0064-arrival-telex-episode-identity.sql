-- Keep existing strip-based keys readable for flights persisted before AMAN
-- assigned an arrival episode ID.
ALTER TABLE arrival_telex_messages
    ALTER COLUMN arrival_episode TYPE TEXT USING arrival_episode::TEXT;
