CREATE TABLE arrival_eta_revisions (
    session_id INTEGER PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    revision BIGINT NOT NULL DEFAULT 0
);

INSERT INTO arrival_eta_revisions(session_id)
SELECT id FROM sessions;

CREATE FUNCTION advance_arrival_eta_revision() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        IF NEW.session = OLD.session AND NEW.arrival_eta IS NOT DISTINCT FROM OLD.arrival_eta THEN
            RETURN NULL;
        END IF;
    END IF;
    IF TG_OP <> 'INSERT' AND EXISTS (SELECT 1 FROM sessions WHERE id = OLD.session) THEN
        INSERT INTO arrival_eta_revisions(session_id, revision) VALUES (OLD.session, 1)
        ON CONFLICT (session_id) DO UPDATE SET revision = arrival_eta_revisions.revision + 1;
    END IF;
    IF TG_OP = 'INSERT' OR (TG_OP = 'UPDATE' AND NEW.session <> OLD.session) THEN
        INSERT INTO arrival_eta_revisions(session_id, revision) VALUES (NEW.session, 1)
        ON CONFLICT (session_id) DO UPDATE SET revision = arrival_eta_revisions.revision + 1;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER strips_arrival_eta_revision
AFTER INSERT OR DELETE OR UPDATE OF arrival_eta, session ON strips
FOR EACH ROW EXECUTE FUNCTION advance_arrival_eta_revision();
