ALTER TABLE strips ADD COLUMN board_state JSONB;
-- An airport board row keeps its stable strips.id, callsign/session uniqueness,
-- existing order and version. Generic ES/VATSIM upserts do not replace this state.
ALTER TABLE strips ADD CONSTRAINT board_state_object CHECK (board_state IS NULL OR jsonb_typeof(board_state) = 'object');
