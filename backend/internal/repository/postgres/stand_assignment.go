package postgres

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type standAssignmentRepository struct {
	queries *database.Queries
}

// NewStandAssignmentRepository creates a repository for SAT assignments and
// stand blocks.
func NewStandAssignmentRepository(db *pgxpool.Pool) *standAssignmentRepository {
	return &standAssignmentRepository{queries: database.New(db)}
}

func standAssignmentToModel(db database.StandAssignment) *models.StandAssignment {
	return &models.StandAssignment{
		ID:             db.ID,
		SessionID:      db.SessionID,
		Callsign:       db.Callsign,
		Stand:          db.Stand,
		Direction:      db.Direction,
		Stage:          db.Stage,
		Source:         db.Source,
		RuleID:         db.RuleID,
		Tier:           db.Tier,
		MatchedVariant: db.MatchedVariant,
		ConflictReason: db.ConflictReason,
		ETA:            PgTimestamptzToTime(db.Eta),
		ETASource:      db.EtaSource,
		AssignedAt:     PgTimestamptzToTime(db.AssignedAt),
		ExpiresAt:      PgTimestamptzToTime(db.ExpiresAt),
		Manual:         db.Manual,
		Acknowledged:   db.Acknowledged,
		AcknowledgedAt: PgTimestamptzToTime(db.AcknowledgedAt),
		AcknowledgedBy: db.AcknowledgedBy,
		VatsimCID:      db.VatsimCid,
		VatsimRevision: db.VatsimRevision,
		Version:        db.Version,
		CreatedAt:      timestampValue(db.CreatedAt),
		UpdatedAt:      timestampValue(db.UpdatedAt),
	}
}

func standBlockToModel(db database.StandBlock) *models.StandBlock {
	return &models.StandBlock{
		ID:        db.ID,
		SessionID: db.SessionID,
		Stand:     db.Stand,
		BlockType: db.BlockType,
		Source:    db.Source,
		Reason:    db.Reason,
		Callsign:  db.Callsign,
		CreatedBy: db.CreatedBy,
		ExpiresAt: PgTimestamptzToTime(db.ExpiresAt),
		Manual:    db.Manual,
		Version:   db.Version,
		CreatedAt: timestampValue(db.CreatedAt),
		UpdatedAt: timestampValue(db.UpdatedAt),
	}
}

func timestampValue(ts pgtype.Timestamptz) time.Time {
	if !ts.Valid {
		return time.Time{}
	}
	return ts.Time
}

func standAssignmentParams(assignment *models.StandAssignment) database.CreateStandAssignmentParams {
	return database.CreateStandAssignmentParams{
		SessionID:      assignment.SessionID,
		Callsign:       assignment.Callsign,
		Stand:          assignment.Stand,
		Direction:      assignment.Direction,
		Stage:          assignment.Stage,
		Source:         assignment.Source,
		RuleID:         assignment.RuleID,
		Tier:           assignment.Tier,
		MatchedVariant: assignment.MatchedVariant,
		ConflictReason: assignment.ConflictReason,
		Eta:            TimeToPgTimestamptz(assignment.ETA),
		EtaSource:      assignment.ETASource,
		AssignedAt:     TimeToPgTimestamptz(assignment.AssignedAt),
		ExpiresAt:      TimeToPgTimestamptz(assignment.ExpiresAt),
		Manual:         assignment.Manual,
		Acknowledged:   assignment.Acknowledged,
		AcknowledgedAt: TimeToPgTimestamptz(assignment.AcknowledgedAt),
		AcknowledgedBy: assignment.AcknowledgedBy,
		VatsimCid:      assignment.VatsimCID,
		VatsimRevision: assignment.VatsimRevision,
	}
}

func standBlockParams(block *models.StandBlock) database.CreateStandBlockParams {
	return database.CreateStandBlockParams{
		SessionID: block.SessionID,
		Stand:     block.Stand,
		BlockType: block.BlockType,
		Source:    block.Source,
		Reason:    block.Reason,
		Callsign:  block.Callsign,
		CreatedBy: block.CreatedBy,
		ExpiresAt: TimeToPgTimestamptz(block.ExpiresAt),
		Manual:    block.Manual,
	}
}

func (r *standAssignmentRepository) CreateAssignment(ctx context.Context, assignment *models.StandAssignment) error {
	created, err := r.queries.CreateStandAssignment(ctx, standAssignmentParams(assignment))
	if err != nil {
		return err
	}
	*assignment = *standAssignmentToModel(created)
	return nil
}

func (r *standAssignmentRepository) GetAssignment(ctx context.Context, session int32, callsign string) (*models.StandAssignment, error) {
	assignment, err := r.queries.GetStandAssignment(ctx, database.GetStandAssignmentParams{SessionID: session, Callsign: callsign})
	if err != nil {
		return nil, err
	}
	return standAssignmentToModel(assignment), nil
}

func (r *standAssignmentRepository) ListAssignments(ctx context.Context, session int32) ([]*models.StandAssignment, error) {
	rows, err := r.queries.ListStandAssignments(ctx, session)
	if err != nil {
		return nil, err
	}
	result := make([]*models.StandAssignment, len(rows))
	for i, row := range rows {
		result[i] = standAssignmentToModel(row)
	}
	return result, nil
}

// LockAssignments locks session assignments for the duration of the caller's
// transaction. It is used exclusively by the SAT allocator while calculating
// availability and choosing an atomic replacement.
func (r *standAssignmentRepository) LockAssignments(ctx context.Context, session int32, callsign string) ([]*models.StandAssignment, error) {
	rows, err := r.queries.LockStandAssignments(ctx, database.LockStandAssignmentsParams{SessionID: session, Callsign: callsign})
	if err != nil {
		return nil, err
	}
	result := make([]*models.StandAssignment, len(rows))
	for i, row := range rows {
		result[i] = standAssignmentToModel(row)
	}
	return result, nil
}

func (r *standAssignmentRepository) UpdateAssignment(ctx context.Context, assignment *models.StandAssignment) (int64, error) {
	return r.queries.UpdateStandAssignment(ctx, database.UpdateStandAssignmentParams{
		ID:             assignment.ID,
		SessionID:      assignment.SessionID,
		Stand:          assignment.Stand,
		Direction:      assignment.Direction,
		Stage:          assignment.Stage,
		Source:         assignment.Source,
		RuleID:         assignment.RuleID,
		Tier:           assignment.Tier,
		MatchedVariant: assignment.MatchedVariant,
		ConflictReason: assignment.ConflictReason,
		Eta:            TimeToPgTimestamptz(assignment.ETA),
		EtaSource:      assignment.ETASource,
		AssignedAt:     TimeToPgTimestamptz(assignment.AssignedAt),
		ExpiresAt:      TimeToPgTimestamptz(assignment.ExpiresAt),
		Manual:         assignment.Manual,
		Acknowledged:   assignment.Acknowledged,
		AcknowledgedAt: TimeToPgTimestamptz(assignment.AcknowledgedAt),
		AcknowledgedBy: assignment.AcknowledgedBy,
		VatsimCid:      assignment.VatsimCID,
		VatsimRevision: assignment.VatsimRevision,
		Version:        assignment.Version,
	})
}

func (r *standAssignmentRepository) DeleteAssignment(ctx context.Context, session int32, id int64, version int32) (int64, error) {
	return r.queries.DeleteStandAssignment(ctx, database.DeleteStandAssignmentParams{ID: id, SessionID: session, Version: version})
}

func (r *standAssignmentRepository) CreateBlock(ctx context.Context, block *models.StandBlock) error {
	created, err := r.queries.CreateStandBlock(ctx, standBlockParams(block))
	if err != nil {
		return err
	}
	*block = *standBlockToModel(created)
	return nil
}

func (r *standAssignmentRepository) GetBlock(ctx context.Context, session int32, id int64) (*models.StandBlock, error) {
	block, err := r.queries.GetStandBlock(ctx, database.GetStandBlockParams{ID: id, SessionID: session})
	if err != nil {
		return nil, err
	}
	return standBlockToModel(block), nil
}

func (r *standAssignmentRepository) ListBlocks(ctx context.Context, session int32) ([]*models.StandBlock, error) {
	rows, err := r.queries.ListStandBlocks(ctx, session)
	if err != nil {
		return nil, err
	}
	return standBlocksToModels(rows), nil
}

// LockActiveManualBlocks locks only blocks that can currently affect an
// allocation. Expired blocks are deliberately excluded from availability.
func (r *standAssignmentRepository) LockActiveManualBlocks(ctx context.Context, session int32) ([]*models.StandBlock, error) {
	rows, err := r.queries.LockActiveManualStandBlocks(ctx, session)
	if err != nil {
		return nil, err
	}
	return standBlocksToModels(rows), nil
}

func (r *standAssignmentRepository) ListBlocksByStand(ctx context.Context, session int32, stand string) ([]*models.StandBlock, error) {
	rows, err := r.queries.ListStandBlocksByStand(ctx, database.ListStandBlocksByStandParams{SessionID: session, Stand: stand})
	if err != nil {
		return nil, err
	}
	return standBlocksToModels(rows), nil
}

func standBlocksToModels(rows []database.StandBlock) []*models.StandBlock {
	result := make([]*models.StandBlock, len(rows))
	for i, row := range rows {
		result[i] = standBlockToModel(row)
	}
	return result
}

func (r *standAssignmentRepository) UpdateBlock(ctx context.Context, block *models.StandBlock) (int64, error) {
	return r.queries.UpdateStandBlock(ctx, database.UpdateStandBlockParams{
		ID:        block.ID,
		SessionID: block.SessionID,
		Stand:     block.Stand,
		BlockType: block.BlockType,
		Source:    block.Source,
		Reason:    block.Reason,
		Callsign:  block.Callsign,
		CreatedBy: block.CreatedBy,
		ExpiresAt: TimeToPgTimestamptz(block.ExpiresAt),
		Manual:    block.Manual,
		Version:   block.Version,
	})
}

func (r *standAssignmentRepository) DeleteBlock(ctx context.Context, session int32, id int64, version int32) (int64, error) {
	return r.queries.DeleteStandBlock(ctx, database.DeleteStandBlockParams{ID: id, SessionID: session, Version: version})
}

// WithTx returns a repository backed by the supplied transaction.
func (r *standAssignmentRepository) WithTx(tx pgx.Tx) repository.StandAssignmentRepository {
	return &standAssignmentRepository{queries: r.queries.WithTx(tx)}
}

var _ repository.StandAssignmentRepository = (*standAssignmentRepository)(nil)
