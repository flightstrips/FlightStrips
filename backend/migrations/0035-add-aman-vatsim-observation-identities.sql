CREATE TABLE IF NOT EXISTS aman_vatsim_observation_identities (
    flight_id VARCHAR PRIMARY KEY,
    vatsim_cid VARCHAR NOT NULL,
    current_callsign VARCHAR NOT NULL,
    retired_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_aman_vatsim_observation_identities_active_cid
    ON aman_vatsim_observation_identities (vatsim_cid)
    WHERE retired_at IS NULL;
