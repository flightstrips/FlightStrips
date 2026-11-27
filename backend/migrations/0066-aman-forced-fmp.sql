-- A controller may force FMP authority for an AMAN session while no FMP
-- position is online. At most one holder exists per session; the row is
-- removed when the holder releases it or logs off, or when an FMP logs on.
-- controller_id ties the grant to the holder's logon, so a row left behind
-- by a failed delete is never restored after the holder logs on again.
CREATE TABLE aman_forced_fmp (
    session_id INTEGER PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    airport VARCHAR NOT NULL,
    callsign VARCHAR NOT NULL,
    cid VARCHAR NOT NULL,
    granted_at TIMESTAMPTZ NOT NULL,
    controller_id INTEGER NOT NULL DEFAULT 0
);
