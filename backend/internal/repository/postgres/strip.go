package postgres

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"context"
	"encoding/json"
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

func marshalCdmData(data *models.CdmData) ([]byte, error) {
	return json.Marshal(data.Normalize())
}

func unmarshalCdmData(raw []byte) (*models.CdmData, error) {
	if len(raw) == 0 {
		return (&models.CdmData{}).Normalize(), nil
	}

	var data models.CdmData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}

	return data.Normalize(), nil
}

// stripToModel converts database.Strip to models.Strip
func stripToModel(db database.Strip) (*models.Strip, error) {
	cdmData, err := unmarshalCdmData(db.CdmData)
	if err != nil {
		return nil, err
	}

	return &models.Strip{
		ID:                       db.ID,
		Version:                  db.Version,
		Callsign:                 db.Callsign,
		Session:                  db.Session,
		Origin:                   db.Origin,
		Destination:              db.Destination,
		Alternative:              db.Alternative,
		Route:                    db.Route,
		Remarks:                  db.Remarks,
		AssignedSquawk:           db.AssignedSquawk,
		Squawk:                   db.Squawk,
		Sid:                      db.Sid,
		ClearedAltitude:          db.ClearedAltitude,
		Heading:                  db.Heading,
		AircraftType:             db.AircraftType,
		Runway:                   db.Runway,
		RequestedAltitude:        db.RequestedAltitude,
		Capabilities:             db.Capabilities,
		CommunicationType:        db.CommunicationType,
		AircraftCategory:         db.AircraftCategory,
		Stand:                    db.Stand,
		Sequence:                 db.Sequence,
		State:                    db.State,
		Cleared:                  db.Cleared,
		Owner:                    db.Owner,
		Bay:                      db.Bay,
		PositionLatitude:         db.PositionLatitude,
		PositionLongitude:        db.PositionLongitude,
		PositionAltitude:         db.PositionAltitude,
		CdmData:                  cdmData,
		NextOwners:               db.NextOwners,
		PreviousOwners:           db.PreviousOwners,
		ReleasePoint:             db.ReleasePoint,
		PdcState:                 db.PdcState,
		PdcRequestedAt:           PgTimestampToTime(db.PdcRequestedAt),
		PdcMessageSequence:       db.PdcMessageSequence,
		PdcMessageSent:           PgTimestampToTime(db.PdcMessageSent),
		Marked:                   db.Marked,
		Registration:             db.Registration,
		TrackingController:       db.TrackingController,
		EngineType:               db.EngineType,
		RunwayCleared:            db.RunwayCleared,
		RunwayConfirmed:          db.RunwayConfirmed,
		UnexpectedChangeFields:   db.UnexpectedChangeFields,
		ControllerModifiedFields: db.ControllerModifiedFields,
	}, nil
}

// Create inserts a new strip
func (r *stripRepository) Create(ctx context.Context, strip *models.Strip) error {
	cdmData, err := marshalCdmData(strip.CdmData)
	if err != nil {
		return err
	}

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
		CdmData:            cdmData,
		Registration:       strip.Registration,
		TrackingController: strip.TrackingController,
		EngineType:         strip.EngineType,
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
	strip, err := stripToModel(dbStrip)
	if err != nil {
		return nil, err
	}
	if manual, err := r.queries.GetManualFPLFields(ctx, session, callsign); err == nil {
		strip.IsManual = manual.IsManual
		strip.PersonsOnBoard = manual.PersonsOnBoard
		strip.FplType = manual.FplType
		strip.Language = manual.Language
		strip.HasFP = manual.HasFP
	}
	return strip, nil
}

// List retrieves all strips for a session
func (r *stripRepository) List(ctx context.Context, session int32) ([]*models.Strip, error) {
	dbStrips, err := r.queries.ListStrips(ctx, session)
	if err != nil {
		return nil, err
	}

	// Bulk-fetch manual FPL fields to avoid N+1.
	manualRows, _ := r.queries.ListManualFPLFieldsBySession(ctx, session)
	manualMap := make(map[string]database.ManualFPLFieldsRow, len(manualRows))
	for _, row := range manualRows {
		manualMap[row.Callsign] = row
	}

	strips := make([]*models.Strip, len(dbStrips))
	for i, dbStrip := range dbStrips {
		s, err := stripToModel(dbStrip)
		if err != nil {
			return nil, err
		}
		if m, ok := manualMap[s.Callsign]; ok {
			s.IsManual = m.IsManual
			s.PersonsOnBoard = m.PersonsOnBoard
			s.FplType = m.FplType
			s.Language = m.Language
			s.HasFP = m.HasFP
		}
		strips[i] = s
	}
	return strips, nil
}

// UpdateIFRManualFPLFields sets IFR FPL fields on an existing strip.
func (r *stripRepository) UpdateIFRManualFPLFields(ctx context.Context, session int32, callsign string, destination string, sid *string, assignedSquawk *string, eobt *string, aircraftType *string, requestedAltitude *int32, route *string, stand *string, runway *string) (int64, error) {
	return r.queries.UpdateIFRManualFPLFields(ctx, database.UpdateIFRManualFPLFieldsParams{
		Session:           session,
		Callsign:          callsign,
		Destination:       destination,
		Sid:               sid,
		AssignedSquawk:    assignedSquawk,
		Eobt:              eobt,
		AircraftType:      aircraftType,
		RequestedAltitude: requestedAltitude,
		Route:             route,
		Stand:             stand,
		Runway:            runway,
	})
}

// UpdateVFRManualFPLFields sets VFR FPL fields and moves the strip to the given bay.
func (r *stripRepository) UpdateVFRManualFPLFields(ctx context.Context, session int32, callsign string, aircraftType *string, personsOnBoard *int32, assignedSquawk string, fplType *string, language *string, remarks *string, bay string) (int64, error) {
	return r.queries.UpdateVFRManualFPLFields(ctx, database.UpdateVFRManualFPLFieldsParams{
		Session:        session,
		Callsign:       callsign,
		AircraftType:   aircraftType,
		PersonsOnBoard: personsOnBoard,
		AssignedSquawk: assignedSquawk,
		FplType:        fplType,
		Language:       language,
		Remarks:        remarks,
		Bay:            bay,
	})
}

// SetHasFP sets the has_fp flag on a strip.
func (r *stripRepository) SetHasFP(ctx context.Context, session int32, callsign string, hasFP bool) error {
	return r.queries.SetHasFP(ctx, session, callsign, hasFP)
}

// Update updates an existing strip
func (r *stripRepository) Update(ctx context.Context, strip *models.Strip) (int64, error) {
	cdmData, err := marshalCdmData(strip.CdmData)
	if err != nil {
		return 0, err
	}

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
		CdmData:            cdmData,
		Registration:       strip.Registration,
		TrackingController: strip.TrackingController,
		EngineType:         strip.EngineType,
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
		strip, err := stripToModel(dbStrip)
		if err != nil {
			return nil, err
		}
		strips[i] = strip
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

// UpdateBayAndSequence atomically updates both the bay and sequence of a strip.
func (r *stripRepository) UpdateBayAndSequence(ctx context.Context, session int32, callsign string, bay string, sequence int32) (int64, error) {
	return r.queries.UpdateStripBayAndSequence(ctx, database.UpdateStripBayAndSequenceParams{
		Session:  session,
		Callsign: callsign,
		Bay:      bay,
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

// GetPrevSequence retrieves the largest sequence below seq in a bay, excluding a callsign
func (r *stripRepository) GetPrevSequence(ctx context.Context, session int32, bay string, seq int32, excludeCallsign string) (int32, error) {
	return r.queries.GetPrevSequence(ctx, database.GetPrevSequenceParams{
		Session:         session,
		Bay:             bay,
		Seq:             seq,
		ExcludeCallsign: excludeCallsign,
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

// UpdateBay updates the bay of a strip
func (r *stripRepository) UpdateBay(ctx context.Context, session int32, callsign string, bay string, version *int32) (int64, error) {
	return r.queries.UpdateStripBayByID(ctx, database.UpdateStripBayByIDParams{
		Bay:      bay,
		Callsign: callsign,
		Session:  session,
		Version:  version,
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

func (r *stripRepository) UpdateRunway(ctx context.Context, session int32, callsign string, runway *string, version *int32) (int64, error) {
	return r.queries.UpdateStripRunwayByID(ctx, database.UpdateStripRunwayByIDParams{
		Runway:   runway,
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
func (r *stripRepository) GetCdmData(ctx context.Context, session int32) ([]*models.CdmDataRow, error) {
	rows, err := r.queries.GetCdmData(ctx, session)
	if err != nil {
		return nil, err
	}

	result := make([]*models.CdmDataRow, len(rows))

	for i, row := range rows {
		cdmData, err := unmarshalCdmData(row.CdmData)
		if err != nil {
			return nil, err
		}
		result[i] = &models.CdmDataRow{
			Callsign: row.Callsign,
			Data:     cdmData,
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

	result, err := unmarshalCdmData(row.CdmData)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SetCdmData replaces the persisted CDM document for a strip.
func (r *stripRepository) SetCdmData(ctx context.Context, session int32, callsign string, data *models.CdmData) (int64, error) {
	cdmData, err := marshalCdmData(data)
	if err != nil {
		return 0, err
	}

	return r.queries.SetCdmData(ctx, database.SetCdmDataParams{
		Session:  session,
		Callsign: callsign,
		CdmData:  cdmData,
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

// UpdateRegistration updates the registration for a strip.
func (r *stripRepository) UpdateRegistration(ctx context.Context, session int32, callsign string, registration string) error {
	_, err := r.queries.UpdateStripRegistration(ctx, database.UpdateStripRegistrationParams{
		Registration: &registration,
		Callsign:     callsign,
		Session:      session,
	})
	return err
}

// UpdateTrackingController updates the tracking_controller field for a strip.
func (r *stripRepository) UpdateTrackingController(ctx context.Context, session int32, callsign string, trackingController string) (int64, error) {
	return r.queries.UpdateTrackingController(ctx, database.UpdateTrackingControllerParams{
		TrackingController: trackingController,
		Callsign:           callsign,
		Session:            session,
	})
}

// AppendUnexpectedChangeField marks a field as unexpectedly changed on a strip.
func (r *stripRepository) AppendUnexpectedChangeField(ctx context.Context, session int32, callsign string, fieldName string) error {
	return r.queries.AppendUnexpectedChangeField(ctx, database.AppendUnexpectedChangeFieldParams{
		Session:     session,
		Callsign:    callsign,
		ArrayAppend: fieldName,
	})
}

// AppendControllerModifiedField marks a field as controller-modified on a strip.
func (r *stripRepository) AppendControllerModifiedField(ctx context.Context, session int32, callsign string, fieldName string) error {
	return r.queries.AppendControllerModifiedField(ctx, database.AppendControllerModifiedFieldParams{
		Session:     session,
		Callsign:    callsign,
		ArrayAppend: fieldName,
	})
}

// RemoveUnexpectedChangeField clears the unexpected-change marker for a field on a strip.
func (r *stripRepository) RemoveUnexpectedChangeField(ctx context.Context, session int32, callsign string, fieldName string) error {
	return r.queries.RemoveUnexpectedChangeField(ctx, database.RemoveUnexpectedChangeFieldParams{
		Session:     session,
		Callsign:    callsign,
		ArrayRemove: fieldName,
	})
}

// UpdateRunwayClearance moves a strip from DEPART to RWY_DEP (if applicable) and sets runway_cleared = true.
// It also auto-confirms (runway_confirmed = true) if no other confirmed strips exist in the session.
func (r *stripRepository) UpdateRunwayClearance(ctx context.Context, session int32, callsign string) (int64, error) {
	return r.queries.UpdateRunwayClearance(ctx, database.UpdateRunwayClearanceParams{
		Callsign: callsign,
		Session:  session,
	})
}

// UpdateRunwayConfirmation marks a strip as runway-confirmed (green indicator).
func (r *stripRepository) UpdateRunwayConfirmation(ctx context.Context, session int32, callsign string) (int64, error) {
	return r.queries.UpdateRunwayConfirmation(ctx, database.UpdateRunwayConfirmationParams{
		Callsign: callsign,
		Session:  session,
	})
}

// ResetRunwayClearance clears runway_cleared back to false (e.g. when a strip is moved backward from rwy-dep).
func (r *stripRepository) ResetRunwayClearance(ctx context.Context, session int32, callsign string) (int64, error) {
	return r.queries.ResetRunwayClearance(ctx, database.ResetRunwayClearanceParams{
		Callsign: callsign,
		Session:  session,
	})
}
