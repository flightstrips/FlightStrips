-- ESTIMATED arrivals now always defer overlaps with later-stage arrivals until
-- their own promotion resolves them, so per-assignment sharing consent is no
-- longer tracked.
DROP TRIGGER clear_retained_stand_estimates ON stand_assignments;
DROP FUNCTION clear_retained_stand_estimates();
DROP TABLE stand_retained_estimates;
