-- Give existing arrivals a stable key. Use the initial telex's timestamp to
-- match old history, since a stand update can outlive the leg that sent it.
WITH legacy_telex AS (
    SELECT session_id, upper(trim(callsign)) AS callsign, arrival_episode, sent_at, id
    FROM arrival_telex_messages
    WHERE kind = 'arrival_info' AND arrival_episode ~ '^[0-9]+$'
), disposition_audits AS MATERIALIZED (
    SELECT audit.session_id, audit.airport, audit.id, audit.payload->>'Category' AS category,
        convert_from(decode(audit.payload->>'Payload', 'base64'), 'UTF8')::jsonb AS facts
    FROM aman_session_audit_records AS audit
    WHERE audit.payload->>'Category' IN ('aman.remove_flight', 'aman.resume_flight')
      AND EXISTS (SELECT 1 FROM legacy_telex AS telex WHERE telex.session_id = audit.session_id)
), flight_legs AS (
    SELECT state.session_id, state.airport, flight.value, flight.ordinality,
        upper(trim(flight.value->>'Callsign')) AS callsign,
        -- Use only confirmed leg boundaries, including when the strip was replaced.
        -- Takeoff refreshes and later lifecycle phases can follow a valid telex.
        GREATEST((flight.value->>'LastGroundedAt')::timestamptz,
            (flight.value->>'PrefileResetAt')::timestamptz,
            CASE WHEN flight.value#>>'{Lifecycle,Reason}' = 'airborne_detected' AND resumed.removed_at IS NULL
                THEN (flight.value#>>'{Lifecycle,EnteredAt}')::timestamptz END) AS confirmed_start,
        resumed.removed_at
    FROM aman_session_states AS state
    CROSS JOIN LATERAL jsonb_array_elements(
        NULLIF(state.payload->'Flights', 'null'::jsonb)) WITH ORDINALITY AS flight
    LEFT JOIN LATERAL (
        -- A legacy manual resume rebuilt the aggregate, so its airborne entry
        -- timestamp dates the command. Match this entry to the remove/resume pair.
        SELECT (removed.facts->>'received_at')::timestamptz AS removed_at
        FROM disposition_audits AS resume
        JOIN LATERAL (
            SELECT prior.category, prior.facts
            FROM disposition_audits AS prior
            WHERE prior.session_id = resume.session_id AND prior.airport = resume.airport
              AND prior.facts->>'callsign' = resume.facts->>'callsign'
              AND prior.facts->>'changed' = 'true' AND prior.id < resume.id
            ORDER BY prior.id DESC
            LIMIT 1
        ) AS removed ON removed.category = 'aman.remove_flight'
            AND removed.facts->>'after_removal_reason' = 'manual_removal'
        WHERE flight.value->>'State' = 'airborne' AND flight.value#>>'{Lifecycle,Reason}' = 'airborne_detected'
          AND resume.session_id = state.session_id AND resume.airport = state.airport
          AND resume.facts->>'callsign' = flight.value->>'Callsign'
          AND resume.category = 'aman.resume_flight' AND resume.facts->>'changed' = 'true'
          AND resume.facts->>'before_state' = 'removed' AND resume.facts->>'before_removal_reason' = 'manual_removal'
          AND resume.facts->>'after_state' = 'airborne'
          AND (resume.facts->>'received_at')::timestamptz = (flight.value#>>'{Lifecycle,EnteredAt}')::timestamptz
        ORDER BY resume.id DESC
        LIMIT 1
    ) AS resumed ON true
), flights AS (
    SELECT flight.session_id, flight.airport,
        jsonb_build_object('Flights', jsonb_agg(
            CASE WHEN COALESCE(flight.value->>'ArrivalEpisodeID', '') = '' THEN
                jsonb_set(flight.value, '{ArrivalEpisodeID}', to_jsonb(
                    COALESCE(history.arrival_episode, 'legacy/' || flight.callsign)))
            ELSE flight.value END ORDER BY flight.ordinality)) AS patch
    FROM flight_legs AS flight
    LEFT JOIN strips AS strip ON strip.session = flight.session_id AND strip.callsign = flight.callsign
    LEFT JOIN LATERAL (
        SELECT telex.arrival_episode
        FROM legacy_telex AS telex
        WHERE COALESCE(flight.value->>'ArrivalEpisodeID', '') = ''
          AND (flight.value->>'State' IN ('unstable', 'stable', 'go_around', 'removed') OR flight.removed_at IS NOT NULL)
          AND telex.session_id = flight.session_id AND telex.callsign = flight.callsign
          AND telex.sent_at >= COALESCE(flight.confirmed_start, '-infinity'::timestamptz)
          AND (flight.removed_at IS NULL OR telex.sent_at <= flight.removed_at)
        ORDER BY (telex.arrival_episode = strip.id::text) DESC NULLS LAST, telex.sent_at DESC, telex.id DESC
        LIMIT 1
    ) AS history ON true
    GROUP BY flight.session_id, flight.airport
    HAVING bool_or(COALESCE(flight.value->>'ArrivalEpisodeID', '') = '')
), disconnected AS (
    SELECT state.session_id, state.airport,
        jsonb_build_object('DisconnectedArrivals', jsonb_object_agg(arrival.key,
            CASE WHEN COALESCE(arrival.value->>'EpisodeID', '') = '' THEN
                jsonb_set(arrival.value, '{EpisodeID}', to_jsonb('legacy/' || upper(trim(arrival.key))))
            ELSE arrival.value END)) AS patch
    FROM aman_session_states AS state
    CROSS JOIN LATERAL jsonb_each(NULLIF(state.payload->'DisconnectedArrivals', 'null'::jsonb)) AS arrival
    GROUP BY state.session_id, state.airport
    HAVING bool_or(COALESCE(arrival.value->>'EpisodeID', '') = '')
)
UPDATE aman_session_states AS state
SET payload = state.payload || COALESCE(flights.patch, '{}'::jsonb) || COALESCE(disconnected.patch, '{}'::jsonb)
FROM flights FULL JOIN disconnected USING (session_id, airport)
WHERE state.session_id = COALESCE(flights.session_id, disconnected.session_id)
  AND state.airport = COALESCE(flights.airport, disconnected.airport);

-- Recover pruned arrivals only when the disappearance audit names the exact
-- episode. A callsign and surviving strip can belong to a successor leg.
WITH removal AS MATERIALIZED (
    SELECT audit.session_id, audit.airport, audit.id,
        convert_from(decode(audit.payload->>'Payload', 'base64'), 'UTF8')::jsonb AS facts
    FROM aman_session_audit_records AS audit
    WHERE audit.payload->>'Category' = 'aman.disconnected_flight_removed'
      AND EXISTS (SELECT 1 FROM arrival_telex_messages AS telex WHERE telex.session_id = audit.session_id)
), latest_removal AS (
    SELECT DISTINCT ON (session_id, airport, upper(trim(facts->>'callsign')))
        session_id, airport, upper(trim(facts->>'callsign')) AS callsign, facts
    FROM removal
    ORDER BY session_id, airport, upper(trim(facts->>'callsign')), id DESC
), recovered AS (
    SELECT state.session_id, state.airport, jsonb_object_agg(removal.callsign, jsonb_build_object(
        'EpisodeID', telex.arrival_episode,
        'Origin', COALESCE(removal.facts->>'origin', ''),
        'ConsumedPrefileDepartureAt', removal.facts->'consumed_prefile_departure_at',
        'MissingSince', removal.facts->'missing_since',
        'RemovedAt', removal.facts->'removed_at')) AS arrivals
    FROM latest_removal AS removal
    JOIN aman_session_states AS state USING (session_id, airport)
    JOIN arrival_telex_messages AS telex ON telex.session_id = removal.session_id
        AND telex.arrival_episode = removal.facts->>'arrival_episode_id' AND upper(trim(telex.callsign)) = removal.callsign
        AND telex.kind = 'arrival_info' AND telex.sent_at <= (removal.facts->>'removed_at')::timestamptz
    WHERE removal.facts->>'flight_state' IN ('airborne', 'unstable', 'stable', 'go_around')
      AND NOT COALESCE(state.payload->'DisconnectedArrivals' ? removal.callsign, false)
      AND NOT EXISTS (
          SELECT 1 FROM jsonb_array_elements(NULLIF(state.payload->'Flights', 'null'::jsonb)) AS flight
          WHERE upper(trim(flight.value->>'Callsign')) = removal.callsign)
    GROUP BY state.session_id, state.airport
)
UPDATE aman_session_states AS state
SET payload = jsonb_set(state.payload, '{DisconnectedArrivals}',
    COALESCE(NULLIF(state.payload->'DisconnectedArrivals', 'null'::jsonb), '{}'::jsonb) || recovered.arrivals)
FROM recovered
WHERE state.session_id = recovered.session_id AND state.airport = recovered.airport;
