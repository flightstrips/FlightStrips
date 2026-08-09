ALTER TABLE stand_assignments
    ADD COLUMN observed_stand VARCHAR;

-- Existing controller assignments predate physical-observation tracking. An
-- empty value marks them for one safe baseline observation after deployment;
-- newly created assignments continue to use NULL when no stand was observed.
UPDATE stand_assignments
SET observed_stand = ''
WHERE manual = TRUE;
