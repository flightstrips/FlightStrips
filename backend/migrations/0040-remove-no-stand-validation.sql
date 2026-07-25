-- Stand assignment is informational and must never lock a strip.
UPDATE strips
SET validation_status = NULL
WHERE validation_status->>'issue_type' = 'NO STAND';
