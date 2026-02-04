
/*
CREATE TABLE IF NOT EXISTS pdc (
    id SERIAL PRIMARY KEY,
    session integer REFERENCES sessions(id) ON DELETE CASCADE NOT NULL,
    pdc_sequence integer NOT NULL DEFAULT 0,
    message_sequence integer NOT NULL DEFAULT 0
)
*/

ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS pdc_sequence integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS pdc_message_sequence integer NOT NULL DEFAULT 0;

ALTER TABLE strips
    ADD COLUMN IF NOT EXISTS pdc_state varchar(255) NOT NULL DEFAULT 'NONE',
    ADD COLUMN IF NOT EXISTS pdc_requested_at timestamp,
    ADD COLUMN IF NOT EXISTS pdc_message_sequence integer,
    ADD COLUMN IF NOT EXISTS pdc_message_sent timestamp;

