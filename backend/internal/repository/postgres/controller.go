package postgres

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type controllerRepository struct {
	queries *database.Queries
}

// NewControllerRepository creates a new ControllerRepository implementation
func NewControllerRepository(db *pgxpool.Pool) *controllerRepository {
	return &controllerRepository{
		queries: database.New(db),
	}
}

// controllerToModel converts database.Controller to models.Controller
func controllerToModel(db database.Controller) *models.Controller {
	return &models.Controller{
		ID:                db.ID,
		Session:           db.Session,
		Callsign:          db.Callsign,
		Position:          db.Position,
		Cid:               db.Cid,
		LastSeenEuroscope: PgTimestampToTime(db.LastSeenEuroscope),
		LastSeenFrontend:  PgTimestampToTime(db.LastSeenFrontend),
		Layout:            db.Layout,
	}
}

// Create inserts a new controller
func (r *controllerRepository) Create(ctx context.Context, controller *models.Controller) error {
	return r.queries.InsertController(ctx, database.InsertControllerParams{
		Callsign:          controller.Callsign,
		Session:           controller.Session,
		Position:          controller.Position,
		Cid:               controller.Cid,
		LastSeenEuroscope: TimeToPgTimestamp(controller.LastSeenEuroscope),
		LastSeenFrontend:  TimeToPgTimestamp(controller.LastSeenFrontend),
	})
}

// Get retrieves a controller by callsign and session
func (r *controllerRepository) Get(ctx context.Context, callsign string, session int32) (*models.Controller, error) {
	dbController, err := r.queries.GetController(ctx, database.GetControllerParams{
		Callsign: callsign,
		Session:  session,
	})
	if err != nil {
		return nil, err
	}
	return controllerToModel(dbController), nil
}

// GetByCid retrieves a controller by CID
func (r *controllerRepository) GetByCid(ctx context.Context, cid string) (*models.Controller, error) {
	dbController, err := r.queries.GetControllerByCid(ctx, cid)
	if err != nil {
		return nil, err
	}
	return controllerToModel(dbController), nil
}

// GetByCallsign retrieves a controller by callsign and session
func (r *controllerRepository) GetByCallsign(ctx context.Context, session int32, callsign string) (*models.Controller, error) {
	dbController, err := r.queries.GetController(ctx, database.GetControllerParams{
		Callsign: callsign,
		Session:  session,
	})
	if err != nil {
		return nil, err
	}
	return controllerToModel(dbController), nil
}

// List retrieves all controllers for a session
func (r *controllerRepository) List(ctx context.Context, session int32) ([]*models.Controller, error) {
	dbControllers, err := r.queries.ListControllers(ctx, session)
	if err != nil {
		return nil, err
	}

	controllers := make([]*models.Controller, len(dbControllers))
	for i, dbController := range dbControllers {
		controllers[i] = controllerToModel(dbController)
	}
	return controllers, nil
}

// ListBySession is an alias for List
func (r *controllerRepository) ListBySession(ctx context.Context, session int32) ([]*models.Controller, error) {
	return r.List(ctx, session)
}

// Delete removes a controller by callsign and session
func (r *controllerRepository) Delete(ctx context.Context, session int32, callsign string) error {
	_, err := r.queries.RemoveController(ctx, database.RemoveControllerParams{
		Callsign: callsign,
		Session:  session,
	})
	return err
}

// SetPosition sets the position of a controller
func (r *controllerRepository) SetPosition(ctx context.Context, session int32, callsign string, position string) (int64, error) {
	return r.queries.SetControllerPosition(ctx, database.SetControllerPositionParams{
		Position: position,
		Callsign: callsign,
		Session:  session,
	})
}

// SetCid sets the CID of a controller
func (r *controllerRepository) SetCid(ctx context.Context, session int32, callsign string, cid *string) (int64, error) {
	return r.queries.SetControllerCid(ctx, database.SetControllerCidParams{
		Cid:      cid,
		Callsign: callsign,
		Session:  session,
	})
}

// SetLayout sets the layout for controllers at a position
func (r *controllerRepository) SetLayout(ctx context.Context, session int32, callsign string, layout *string) (int64, error) {
	// Note: The SQL query sets layout by position, but we need to get the position first
	controller, err := r.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return 0, err
	}

	return r.queries.SetControllerLayout(ctx, database.SetControllerLayoutParams{
		Layout:   layout,
		Position: controller.Position,
		Session:  session,
	})
}

// SetEuroscopeSeen updates the last_seen_euroscope timestamp for a controller by CID
func (r *controllerRepository) SetEuroscopeSeen(ctx context.Context, cid string, session int32, lastSeen *time.Time) (int64, error) {
	return r.queries.SetControllerEuroscopeSeen(ctx, database.SetControllerEuroscopeSeenParams{
		Cid:               cid,
		Session:           session,
		LastSeenEuroscope: TimeToPgTimestamp(lastSeen),
	})
}

// SetFrontendSeen updates the last_seen_frontend timestamp for a controller by CID
func (r *controllerRepository) SetFrontendSeen(ctx context.Context, cid string, session int32, lastSeen *time.Time) (int64, error) {
	return r.queries.SetControllerFrontendSeen(ctx, database.SetControllerFrontendSeenParams{
		Cid:              cid,
		Session:          session,
		LastSeenFrontend: TimeToPgTimestamp(lastSeen),
	})
}
