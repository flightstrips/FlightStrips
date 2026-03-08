ALTER TABLE coordinations
    ADD COLUMN from_es         BOOLEAN      NOT NULL DEFAULT FALSE,
    ADD COLUMN es_handover_cid VARCHAR(255);
