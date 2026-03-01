package postgres

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type stripRepository struct {
	queries *database.Queries
}

// NewStripRepository creates a new StripRepository implementation
func NewStripRepository(db *pgxpool.Pool) *stripRepository {
	return &stripRepository{
		queries: database.New(db),
	}
}

// stripToModel converts database.Strip to models.Strip
func stripToModel(db database.Strip) *models.Strip {
	return &models.Strip{
		ID:                 db.ID,
		Version:            db.Version,
		Callsign:           db.Callsign,
		Session:            db.Session,
		Origin:             db.Origin,
		Destination:        db.Destination,
		Alternative:        db.Alternative,
		Route:              db.Route,
		Remarks:            db.Remarks,
		AssignedSquawk:     db.AssignedSquawk,
		Squawk:             db.Squawk,
		Sid:                db.Sid,
		ClearedAltitude:    db.ClearedAltitude,
		Heading:            db.Heading,
		AircraftType:       db.AircraftType,
		Runway:             db.Runway,
		RequestedAltitude:  db.RequestedAltitude,
		Capabilities:       db.Capabilities,
		CommunicationType:  db.CommunicationType,
		AircraftCategory:   db.AircraftCategory,
		Stand:              db.Stand,
		Sequence:           db.Sequence,
		State:              db.State,
		Cleared:            db.Cleared,
		Owner:              db.Owner,
		Bay:                db.Bay,
		PositionLatitude:   db.PositionLatitude,
		PositionLongitude:  db.PositionLongitude,
		PositionAltitude:   db.PositionAltitude,
		Tobt:               db.Tobt,
		Tsat:               db.Tsat,
		Ttot:               db.Ttot,
		Ctot:               db.Ctot,
		Aobt:               db.Aobt,
		Asat:               db.Asat,
		Eobt:               db.Eobt,
		NextOwners:         db.NextOwners,
		PreviousOwners:     db.PreviousOwners,
		CdmStatus:          db.CdmStatus,
		ReleasePoint:       db.ReleasePoint,
		PdcState:           db.PdcState,
		PdcRequestedAt:     PgTimestampToTime(db.PdcRequestedAt),
		PdcMessageSequence: db.PdcMessageSequence,
		PdcMessageSent:     PgTimestampToTime(db.PdcMessageSent),
		Marked:             db.Marked,
	}
}

// Create inserts a new strip
func (r *stripRepository) Create(ctx context.Context, strip *models.Strip) error {
	return r.queries.InsertStrip(ctx, database.InsertStripParams{
		Callsign:           strip.Callsign,
		Session:            strip.Session,
		Origin:             strip.Origin,
		Destination:        strip.Destination,
		Alternative:        strip.Alternative,
		Route:              strip.Route,
		Remarks:            strip.Remarks,
		AssignedSquawk:     strip.AssignedSquawk,
		Squawk:             strip.Squawk,
		Sid:                strip.Sid,
		ClearedAltitude:    strip.ClearedAltitude,
		Heading:            strip.Heading,
		AircraftType:       strip.AircraftType,
		Runway:             strip.Runway,
		RequestedAltitude:  strip.RequestedAltitude,
		Capabilities:       strip.Capabilities,
		CommunicationType:  strip.CommunicationType,
		AircraftCategory:   strip.AircraftCategory,
		Stand:              strip.Stand,
		Sequence:           strip.Sequence,
		State:              strip.State,
		Cleared:            strip.Cleared,
		Owner:              strip.Owner,
		Bay:                strip.Bay,
		PositionLatitude:   strip.PositionLatitude,
		PositionLongitude:  strip.PositionLongitude,
		PositionAltitude:   strip.PositionAltitude,
		Tobt:               strip.Tobt,
		Eobt:               strip.Eobt,
	})
}

// GetByCallsign retrieves a strip by callsign and session
func (r *stripRepository) GetByCallsign(ctx context.Context, session int32, callsign string) (*models.Strip, error) {
	dbStrip, err := r.queries.GetStrip(ctx, database.GetStripParams{
		Session:  session,
		Callsign: callsign,
	})
	if err != nil {
		return nil, err
	}
	return stripToModel(dbStrip), nil
}

// List retrieves all strips for a session
func (r *stripRepository) List(ctx context.Context, session int32) ([]*models.Strip, error) {
	dbStrips, err := r.queries.ListStrips(ctx, session)
	if err != nil {
		return nil, err
	}

	strips := make([]*models.Strip, len(dbStrips))
	for i, dbStrip := range dbStrips {
		strips[i] = stripToModel(dbStrip)
	}
	return strips, nil
}

// Update updates an existing strip
func (r *stripRepository) Update(ctx context.Context, strip *models.Strip) (int64, error) {
	return r.queries.UpdateStrip(ctx, database.UpdateStripParams{
		Callsign:           strip.Callsign,
		Session:            strip.Session,
		Origin:             strip.Origin,
		Destination:        strip.Destination,
		Alternative:        strip.Alternative,
		Route:              strip.Route,
		Remarks:            strip.Remarks,
		AssignedSquawk:     strip.AssignedSquawk,
		Squawk:             strip.Squawk,
		Sid:                strip.Sid,
		ClearedAltitude:    strip.ClearedAltitude,
		Heading:            strip.Heading,
		AircraftType:       strip.AircraftType,
		Runway:             strip.Runway,
		RequestedAltitude:  strip.RequestedAltitude,
		Capabilities:       strip.Capabilities,
		CommunicationType:  strip.CommunicationType,
		AircraftCategory:   strip.AircraftCategory,
		Stand:              strip.Stand,
		Sequence:           strip.Sequence,
		State:              strip.State,
		Cleared:            strip.Cleared,
		Owner:              strip.Owner,
		Bay:                strip.Bay,
		PositionLatitude:   strip.PositionLatitude,
		PositionLongitude:  strip.PositionLongitude,
		PositionAltitude:   strip.PositionAltitude,
		Tobt:               strip.Tobt,
		Eobt:               strip.Eobt,
	})
}

// Delete removes a strip by callsign and session
func (r *stripRepository) Delete(ctx context.Context, session int32, callsign string) error {
	return r.queries.RemoveStripByID(ctx, database.RemoveStripByIDParams{
		Session:  session,
		Callsign: callsign,
	})
}

// ListByOrigin retrieves all strips for a session by origin
func (r *stripRepository) ListByOrigin(ctx context.Context, session int32, origin string) ([]*models.Strip, error) {
	dbStrips, err := r.queries.ListStripsByOrigin(ctx, database.ListStripsByOriginParams{
		Session: session,
		Origin:  origin,
	})
	if err != nil {
		return nil, err
	}

	strips := make([]*models.Strip, len(dbStrips))
	for i, dbStrip := range dbStrips {
		strips[i] = stripToModel(dbStrip)
	}
	return strips, nil
}

// GetBay retrieves the bay for a strip
func (r *stripRepository) GetBay(ctx context.Context, session int32, callsign string) (string, error) {
	return r.queries.GetBay(ctx, database.GetBayParams{
		Session:  session,
		Callsign: callsign,
	})
}

// UpdateSequence updates the sequence of a strip
func (r *stripRepository) UpdateSequence(ctx context.Context, session int32, callsign string, sequence int32) (int64, error) {
	return r.queries.UpdateStripSequence(ctx, database.UpdateStripSequenceParams{
		Session:  session,
		Callsign: callsign,
		Sequence: sequence,
	})
}

// UpdateSequenceBulk updates multiple strip sequences
func (r *stripRepository) UpdateSequenceBulk(ctx context.Context, session int32, callsigns []string, sequences []int32) error {
	return r.queries.UpdateStripSequenceBulk(ctx, database.UpdateStripSequenceBulkParams{
		Session:   session,
		Callsigns: callsigns,
		Sequences: sequences,
	})
}

// RecalculateSequences recalculates all sequences in a bay
func (r *stripRepository) RecalculateSequences(ctx context.Context, session int32, bay string, spacing int32) error {
	return r.queries.RecalculateStripSequences(ctx, database.RecalculateStripSequencesParams{
		Session: session,
		Bay:     bay,
		Spacing: spacing,
	})
}

// ListSequences retrieves all sequences in a bay
func (r *stripRepository) ListSequences(ctx context.Context, session int32, bay string) ([]*models.StripSequence, error) {
	rows, err := r.queries.ListStripSequences(ctx, database.ListStripSequencesParams{
		Session: session,
		Bay:     bay,
	})
	if err != nil {
		return nil, err
	}

	result := make([]*models.StripSequence, len(rows))
	for i, row := range rows {
		result[i] = &models.StripSequence{
			Callsign: row.Callsign,
			Sequence: row.Sequence,
		}
	}
	return result, nil
}

// GetSequence retrieves the sequence for a strip in a bay
func (r *stripRepository) GetSequence(ctx context.Context, session int32, callsign string, bay string) (int32, error) {
	return r.queries.GetSequence(ctx, database.GetSequenceParams{
		Session:  session,
		Callsign: callsign,
		Bay:      bay,
	})
}

// GetMaxSequenceInBay retrieves the maximum sequence in a bay
func (r *stripRepository) GetMaxSequenceInBay(ctx context.Context, session int32, bay string) (int32, error) {
	return r.queries.GetMaxSequenceInBay(ctx, database.GetMaxSequenceInBayParams{
		Session: session,
		Bay:     bay,
	})
}

// GetMinSequenceInBay retrieves the minimum sequence in a bay
func (r *stripRepository) GetMinSequenceInBay(ctx context.Context, session int32, bay string) (int32, error) {
	return r.queries.GetMinSequenceInBay(ctx, database.GetMinSequenceInBayParams{
		Session: session,
		Bay:     bay,
	})
}

// GetNextSequence retrieves the next sequence after the given sequence in a bay
func (r *stripRepository) GetNextSequence(ctx context.Context, session int32, bay string, sequence int32) (int32, error) {
	return r.queries.GetNextSequence(ctx, database.GetNextSequenceParams{
		Session:  session,
		Bay:      bay,
		Sequence: sequence,
	})
}

// UpdateSquawk updates the squawk code of a strip
func (r *stripRepository) UpdateSquawk(ctx context.Context, session int32, callsign string, squawk *string, version *int32) (int64, error) {
	return r.queries.UpdateStripSquawkByID(ctx, database.UpdateStripSquawkByIDParams{
		Squawk:   squawk,
		Callsign: callsign,
		Session:  session,
		Version:  version,
	})
}

// UpdateAssignedSquawk updates the assigned squawk code of a strip
func (r *stripRepository) UpdateAssignedSquawk(ctx context.Context, session int32, callsign string, assignedSquawk *string, version *int32) (int64, error) {
	return r.queries.UpdateStripAssignedSquawkByID(ctx, database.UpdateStripAssignedSquawkByIDParams{
		AssignedSquawk: assignedSquawk,
		Callsign:       callsign,
		Session:        session,
		Version:        version,
	})
}

// UpdateClearedAltitude updates the cleared altitude of a strip
func (r *stripRepository) UpdateClearedAltitude(ctx context.Context, session int32, callsign string, altitude *int32, version *int32) (int64, error) {
	return r.queries.UpdateStripClearedAltitudeByID(ctx, database.UpdateStripClearedAltitudeByIDParams{
		ClearedAltitude: altitude,
		Callsign:        callsign,
		Session:         session,
		Version:         version,
	})
}

// UpdateRequestedAltitude updates the requested altitude of a strip
func (r *stripRepository) UpdateRequestedAltitude(ctx context.Context, session int32, callsign string, altitude *int32, version *int32) (int64, error) {
	return r.queries.UpdateStripRequestedAltitudeByID(ctx, database.UpdateStripRequestedAltitudeByIDParams{
		RequestedAltitude: altitude,
		Callsign:          callsign,
		Session:           session,
		Version:           version,
	})
}

// UpdateCommunicationType updates the communication type of a strip
func (r *stripRepository) UpdateCommunicationType(ctx context.Context, session int32, callsign string, commType *string, version *int32) (int64, error) {
	return r.queries.UpdateStripCommunicationTypeByID(ctx, database.UpdateStripCommunicationTypeByIDParams{
		CommunicationType: commType,
		Callsign:          callsign,
		Session:           session,
		Version:           version,
	})
}

// UpdateGroundState updates the ground state of a strip
func (r *stripRepository) UpdateGroundState(ctx context.Context, session int32, callsign string, state *string, bay string, version *int32) (int64, error) {
	return r.queries.UpdateStripGroundStateByID(ctx, database.UpdateStripGroundStateByIDParams{
		State:    state,
		Bay:      bay,
		Callsign: callsign,
		Session:  session,
		Version:  version,
	})
}

// UpdateClearedFlag updates the cleared flag of a strip
func (r *stripRepository) UpdateClearedFlag(ctx context.Context, session int32, callsign string, cleared bool, bay string, version *int32) (int64, error) {
	return r.queries.UpdateStripClearedFlagByID(ctx, database.UpdateStripClearedFlagByIDParams{
		Cleared:  cleared,
		Bay:      bay,
		Callsign: callsign,
		Session:  session,
		Version:  version,
	})
}

// UpdateAircraftPosition updates the aircraft position of a strip
func (r *stripRepository) UpdateAircraftPosition(ctx context.Context, session int32, callsign string, lat *float64, lon *float64, alt *int32, bay string, version *int32) (int64, error) {
	return r.queries.UpdateStripAircraftPositionByID(ctx, database.UpdateStripAircraftPositionByIDParams{
		PositionLatitude:  lat,
		PositionLongitude: lon,
		PositionAltitude:  alt,
		Bay:               bay,
		Callsign:          callsign,
		Session:           session,
		Version:           version,
	})
}

// UpdateHeading updates the heading of a strip
func (r *stripRepository) UpdateHeading(ctx context.Context, session int32, callsign string, heading *int32, version *int32) (int64, error) {
	return r.queries.UpdateStripHeadingByID(ctx, database.UpdateStripHeadingByIDParams{
		Heading:  heading,
		Callsign: callsign,
		Session:  session,
		Version:  version,
	})
}

// UpdateStand updates the stand of a strip
func (r *stripRepository) UpdateStand(ctx context.Context, session int32, callsign string, stand *string, version *int32) (int64, error) {
	return r.queries.UpdateStripStandByID(ctx, database.UpdateStripStandByIDParams{
		Stand:    stand,
		Callsign: callsign,
		Session:  session,
		Version:  version,
	})
}

// SetOwner sets the owner of a strip
func (r *stripRepository) SetOwner(ctx context.Context, session int32, callsign string, owner *string, version int32) (int64, error) {
	return r.queries.SetStripOwner(ctx, database.SetStripOwnerParams{
		Owner:    owner,
		Callsign: callsign,
		Session:  session,
		Version:  version,
	})
}

// SetNextOwners sets the next owners of a strip
func (r *stripRepository) SetNextOwners(ctx context.Context, session int32, callsign string, nextOwners []string) error {
	return r.queries.SetNextOwners(ctx, database.SetNextOwnersParams{
		Session:    session,
		Callsign:   callsign,
		NextOwners: nextOwners,
	})
}

// SetPreviousOwners sets the previous owners of a strip
func (r *stripRepository) SetPreviousOwners(ctx context.Context, session int32, callsign string, previousOwners []string) error {
	return r.queries.SetPreviousOwners(ctx, database.SetPreviousOwnersParams{
		Session:        session,
		Callsign:       callsign,
		PreviousOwners: previousOwners,
	})
}

// SetNextAndPreviousOwners sets both next and previous owners of a strip
func (r *stripRepository) SetNextAndPreviousOwners(ctx context.Context, session int32, callsign string, nextOwners []string, previousOwners []string) error {
	return r.queries.SetNextAndPreviousOwners(ctx, database.SetNextAndPreviousOwnersParams{
		Session:        session,
		Callsign:       callsign,
		NextOwners:     nextOwners,
		PreviousOwners: previousOwners,
	})
}

// GetCdmData retrieves CDM data for all strips in a session
func (r *stripRepository) GetCdmData(ctx context.Context, session int32) ([]*models.CdmData, error) {
	rows, err := r.queries.GetCdmData(ctx, session)
	if err != nil {
		return nil, err
	}

	result := make([]*models.CdmData, len(rows))

	for i, row := range rows {
		result[i] = &models.CdmData{
			Callsign:  row.Callsign,
			Tobt:      row.Tobt,
			Tsat:      row.Tsat,
			Ttot:      row.Ttot,
			Ctot:      row.Ctot,
			Aobt:      row.Aobt,
			Asat:      row.Asat,
			Eobt:      row.Eobt,
			CdmStatus: row.CdmStatus,
		}
	}
	return result, nil
}

// GetCdmDataForCallsign retrieves CDM data for a specific strip
func (r *stripRepository) GetCdmDataForCallsign(ctx context.Context, session int32, callsign string) (*models.CdmData, error) {
	row, err := r.queries.GetCdmDataForCallsign(ctx, database.GetCdmDataForCallsignParams{
		Session:  session,
		Callsign: callsign,
	})
	if err != nil {
		return nil, err
	}

	result := &models.CdmData{
		Callsign:  row.Callsign,
		Tobt:      row.Tobt,
		Tsat:      row.Tsat,
		Ttot:      row.Ttot,
		Ctot:      row.Ctot,
		Aobt:      row.Aobt,
		Asat:      row.Asat,
		Eobt:      row.Eobt,
		CdmStatus: row.CdmStatus,
	}
	return result, nil
}

// UpdateCdmData updates CDM data for a strip
func (r *stripRepository) UpdateCdmData(ctx context.Context, session int32, callsign string, tobt *string, tsat *string, ttot *string, ctot *string, aobt *string, eobt *string, cdmStatus *string) (int64, error) {
	return r.queries.UpdateCdmData(ctx, database.UpdateCdmDataParams{
		Session:   session,
		Callsign:  callsign,
		Tobt:      tobt,
		Tsat:      tsat,
		Ttot:      ttot,
		Ctot:      ctot,
		Aobt:      aobt,
		Eobt:      eobt,
		CdmStatus: cdmStatus,
	})
}

// SetCdmStatus sets the CDM status of a strip
func (r *stripRepository) SetCdmStatus(ctx context.Context, session int32, callsign string, cdmStatus *string) (int64, error) {
	return r.queries.SetCdmStatus(ctx, database.SetCdmStatusParams{
		Session:   session,
		Callsign:  callsign,
		CdmStatus: cdmStatus,
	})
}

// UpdateReleasePoint updates the release point of a strip
func (r *stripRepository) UpdateReleasePoint(ctx context.Context, session int32, callsign string, releasePoint *string) (int64, error) {
	return r.queries.UpdateReleasePoint(ctx, database.UpdateReleasePointParams{
		Session:      session,
		Callsign:     callsign,
		ReleasePoint: releasePoint,
	})
}

// SetPdcRequested sets PDC requested state and timestamp
func (r *stripRepository) SetPdcRequested(ctx context.Context, session int32, callsign string, pdcState string, pdcRequestedAt *time.Time) error {
	return r.queries.SetPdcRequested(ctx, database.SetPdcRequestedParams{
		Callsign:       callsign,
		Session:        session,
		PdcState:       pdcState,
		PdcRequestedAt: TimeToPgTimestamp(pdcRequestedAt),
	})
}

// SetPdcMessageSent sets PDC message sent state, sequence, and timestamp
func (r *stripRepository) SetPdcMessageSent(ctx context.Context, session int32, callsign string, pdcState string, pdcMessageSequence *int32, pdcMessageSent *time.Time) error {
	return r.queries.SetPdcMessageSent(ctx, database.SetPdcMessageSentParams{
		Callsign:           callsign,
		Session:            session,
		PdcState:           pdcState,
		PdcMessageSequence: pdcMessageSequence,
		PdcMessageSent:     TimeToPgTimestamp(pdcMessageSent),
	})
}

// UpdatePdcStatus updates only the PDC state
func (r *stripRepository) UpdatePdcStatus(ctx context.Context, session int32, callsign string, pdcState string) error {
	return r.queries.UpdatePdcStatus(ctx, database.UpdatePdcStatusParams{
		Callsign: callsign,
		Session:  session,
		PdcState: pdcState,
	})
}

// UpdateMarked updates the marked flag of a strip
func (r *stripRepository) UpdateMarked(ctx context.Context, session int32, callsign string, marked bool, version *int32) (int64, error) {
	return r.queries.UpdateStripMarkedByID(ctx, database.UpdateStripMarkedByIDParams{
		Marked:   marked,
		Callsign: callsign,
		Session:  session,
		Version:  version,
	})
}
