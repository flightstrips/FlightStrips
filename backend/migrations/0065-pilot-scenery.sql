-- The airport scenery a pilot declares in the EFB. Some sceneries lack stands
-- that exist in reality, so stand allocation avoids those for that pilot.
CREATE TABLE pilot_sceneries (
    cid        BIGINT      NOT NULL,
    airport    TEXT        NOT NULL,
    scenery    TEXT        NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (cid, airport)
);
