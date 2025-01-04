-- name: InsertController :exec
INSERT INTO controllers (
    cid, airport, position
) VALUES (
             ?, ?, ?
         );

-- name: ListControllers :many
SELECT * FROM controllers;

-- name: ListControllersByAirport :many
SELECT * FROM controllers WHERE airport = ?;

-- name: RemoveController :exec
DELETE FROM controllers WHERE cid = ?;

-- name: InsertStrip :exec
INSERT INTO strips (
    id, origin, destination, alternative, route, remarks, assigned_squawk, squawk, sid, cleared_altitude, heading, aircraft_type, runway, requested_altitude, capabilities, communication_type, aircraft_category, stand, sequence, state, cleared, positionFrequency, position_latitude, position_longitude, position_altitude, tobt, tsat, ttot, ctot, aobt, asat
) VALUES (
             ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
         );

-- name: UpdateStripByID :exec
UPDATE strips SET (
        origin, destination, alternative, route, remarks, assigned_squawk, squawk, sid, cleared_altitude, heading, aircraft_type, runway, requested_altitude, capabilities, communication_type, aircraft_category, stand, sequence, state, cleared, positionFrequency, position_latitude, position_longitude, position_altitude, tobt, tsat, ttot, ctot, aobt, asat
) = (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
    ) WHERE id = ?;

-- name: RemoveStripByID :exec
DELETE FROM strips WHERE id = ?;

-- name: ListStripsByOrigin :many
SELECT * FROM strips WHERE origin = ?;

