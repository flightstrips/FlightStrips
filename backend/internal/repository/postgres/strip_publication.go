package postgres

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"strings"
)

type stripPublicationRow struct {
	database.Strip
	PublicationSession  []byte
	Controllers         []byte
	SectorOwners        []byte
	Assignments         []byte
	CoordinationPending bool
}

// GetStripPublicationSnapshot reads the complete publication inputs at one
// statement snapshot, without changing transaction or persistence boundaries.
func (r *stripRepository) GetStripPublicationSnapshot(ctx context.Context, session int32, callsign string) (*models.StripPublicationSnapshot, error) {
	rows, err := r.db.Query(ctx, `-- strip publication snapshot
SELECT s.id, s.version, s.callsign, s.session, s.origin, s.destination, s.alternative, s.route, s.remarks,
 s.assigned_squawk, s.squawk, s.sid, s.cleared_altitude, s.heading, s.aircraft_type, s.runway,
 s.requested_altitude, s.capabilities, s.communication_type, s.aircraft_category, s.stand,
 s.sequence, s.state, s.cleared, s.owner, s.bay, s.position_latitude, s.position_longitude,
 s.position_altitude, s.next_owners, s.previous_owners, s.release_point, s.marked, s.registration,
 s.tracking_controller, s.runway_cleared, s.unexpected_change_fields, s.controller_modified_fields,
 s.engine_type, s.is_manual, s.persons_on_board, s.fpl_type, s.language, s.has_fp, s.cdm_data,
 s.runway_confirmed, s.pdc_data, s.validation_status, s.start_req, s.spoken_callsign, s.vatsim_cid,
 s.vatsim_revision, s.vatsim_seen_at, s.euroscope_seen_at, s.arrival_eta, s.star, s.hold, s.hold_type,
 s.hold_eat, s.next_display_label, s.next_display_frequency, s.fs_scratch_pad, s.flight_rules,
 s.release_point_proposal, s.release_point_proposed_by, to_jsonb(se) AS publication_session,
 (SELECT COALESCE(jsonb_agg(c ORDER BY c.callsign), '[]') FROM controllers c WHERE c.session=s.session) AS controllers,
 (SELECT COALESCE(jsonb_agg(o ORDER BY o.id), '[]') FROM sector_owners o WHERE o.session=s.session) AS sector_owners,
 CASE WHEN EXISTS (SELECT 1 FROM stand_assignments a WHERE a.session_id=s.session AND a.callsign=s.callsign)
 THEN (SELECT COALESCE(jsonb_agg(a ORDER BY a.callsign), '[]') FROM stand_assignments a WHERE a.session_id=s.session)
 ELSE '[]'::jsonb END AS assignments,
 EXISTS (SELECT 1 FROM coordinations c WHERE c.session=s.session AND c.strip_id=s.id) AS coordination_pending
FROM strips s JOIN sessions se ON se.id=s.session
WHERE s.session=$1 AND s.callsign=$2`, session, callsign)
	if err != nil {
		return nil, err
	}
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[stripPublicationRow])
	if err != nil {
		return nil, err
	}
	strip, err := stripToModel(row.Strip)
	if err != nil {
		return nil, err
	}
	var se database.Session
	if err = decodeDatabaseJSON(row.PublicationSession, &se); err != nil {
		return nil, err
	}
	controllers, err := decodeDatabaseJSONArray[database.Controller](row.Controllers)
	if err != nil {
		return nil, err
	}
	owners, err := decodeDatabaseJSONArray[database.SectorOwner](row.SectorOwners)
	if err != nil {
		return nil, err
	}
	assignments, err := decodeDatabaseJSONArray[database.StandAssignment](row.Assignments)
	if err != nil {
		return nil, err
	}
	result := &models.StripPublicationSnapshot{Strip: strip, Session: sessionToModel(se), CoordinationPending: row.CoordinationPending}
	for _, c := range controllers {
		result.Controllers = append(result.Controllers, controllerToModel(c))
	}
	for _, o := range owners {
		result.SectorOwners = append(result.SectorOwners, sectorOwnerToModel(o))
	}
	for _, a := range assignments {
		result.Assignments = append(result.Assignments, standAssignmentToModel(a))
	}
	return result, nil
}

// sqlc records have Go field names; normalize only database column names, not
// nested JSON payloads (whose keys are part of their own schema).
func decodeDatabaseJSON(raw []byte, target any) error {
	var columns map[string]json.RawMessage
	if err := json.Unmarshal(raw, &columns); err != nil {
		return err
	}
	fields := make(map[string]json.RawMessage, len(columns))
	for k, v := range columns {
		fields[strings.ReplaceAll(k, "_", "")] = v
	}
	normalized, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	return json.Unmarshal(normalized, target)
}

func decodeDatabaseJSONArray[T any](raw []byte) ([]T, error) {
	var records []json.RawMessage
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, err
	}
	result := make([]T, len(records))
	for i, record := range records {
		if err := decodeDatabaseJSON(record, &result[i]); err != nil {
			return nil, err
		}
	}
	return result, nil
}
