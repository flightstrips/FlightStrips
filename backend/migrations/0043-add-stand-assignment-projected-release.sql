-- A physical departure block has no expiry because observed occupancy remains
-- authoritative until the aircraft actually leaves. Keep its operationally
-- projected release separately so future arrivals can still share the stand.
ALTER TABLE stand_assignments
    ADD COLUMN projected_release_at TIMESTAMPTZ;
