-- Explicit consent for a temporary overlap, scoped to these assignments and stands.
CREATE TABLE stand_retained_estimates (
    estimated_id BIGINT NOT NULL REFERENCES stand_assignments(id) ON DELETE CASCADE,
    manual_id BIGINT NOT NULL REFERENCES stand_assignments(id) ON DELETE CASCADE,
    estimated_stand TEXT NOT NULL,
    manual_stand TEXT NOT NULL,
    PRIMARY KEY (estimated_id, manual_id)
);

-- A later move or promotion ends the consent, even if an assignment moves back.
CREATE FUNCTION clear_retained_stand_estimates() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    DELETE FROM stand_retained_estimates WHERE estimated_id = NEW.id OR manual_id = NEW.id;
    RETURN NEW;
END;
$$;
CREATE TRIGGER clear_retained_stand_estimates
AFTER UPDATE OF stand, stage, manual, direction ON stand_assignments
FOR EACH ROW WHEN (OLD.stand IS DISTINCT FROM NEW.stand OR OLD.stage IS DISTINCT FROM NEW.stage
    OR OLD.manual IS DISTINCT FROM NEW.manual OR OLD.direction IS DISTINCT FROM NEW.direction)
EXECUTE FUNCTION clear_retained_stand_estimates();
