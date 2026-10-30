ALTER TABLE strips
    ADD COLUMN IF NOT EXISTS release_point_proposal varchar(255),
    ADD COLUMN IF NOT EXISTS release_point_proposed_by varchar(255);
