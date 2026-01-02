package cdm

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/shared"
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	client      *Client
	queries     *database.Queries
	frontendHub shared.FrontendHub
}

func NewCdmService(client *Client, dbPool *pgxpool.Pool, frontendHub shared.FrontendHub) *Service {
	return &Service{client: client, queries: database.New(dbPool), frontendHub: frontendHub}
}

func (s *Service) SetReady(ctx context.Context, session int32, callsign string) error {
	cdmData, err := s.queries.GetCdmDataForCallsign(ctx, database.GetCdmDataForCallsignParams{Callsign: callsign, Session: session})
	if err != nil {
		return err
	}

	if cdmData.CdmStatus.Valid && cdmData.CdmStatus.String == "REA" {
		return nil
	}

	if err := s.client.IFPSSetCDMStatus(ctx, callsign, "REA"); err != nil {
		return err
	}

	rows, err := s.queries.SetCdmStatus(ctx, database.SetCdmStatusParams{
		CdmStatus: pgtype.Text{Valid: true, String: "REA"}, Callsign: callsign, Session: session,
	})
	if err != nil {
		return err
	}

	if rows != 1 {
		return fmt.Errorf("failed to update CDM status for %s session %d", callsign, session)
	}

	return nil
}

func (s *Service) RequestBetterTobt(ctx context.Context, session int32, callsign string) error {
	now := time.Now()
	// format hhmm
	format := now.Format("1504")
	status := "REQTOBT/" + format + "/ATC"

	cdmData, err := s.queries.GetCdmDataForCallsign(ctx, database.GetCdmDataForCallsignParams{Callsign: callsign, Session: session})
	if err != nil {
		return err
	}

	if cdmData.CdmStatus.Valid && cdmData.CdmStatus.String == status {
		return nil
	}

	err = s.client.IFPSSetCDMStatus(ctx, callsign, status)
	if err != nil {
		return err
	}

	rows, err := s.queries.SetCdmStatus(ctx, database.SetCdmStatusParams{
		CdmStatus: pgtype.Text{Valid: true, String: status}, Callsign: callsign, Session: session,
	})
	if err != nil {
		return err
	}

	if rows != 1 {
		return fmt.Errorf("failed to update CDM status for %s session %d", callsign, session)
	}

	return nil
}

func (s *Service) SyncCdmData(ctx context.Context, session int32) error {
	sessionData, err := s.queries.GetSessionById(ctx, session)
	if err != nil {
		return err
	}

	airport := sessionData.Airport

	currentData, err := s.queries.GetCdmData(ctx, session)
	if err != nil {
		return err
	}

	lookup := make(map[string]database.GetCdmDataRow)
	for _, row := range currentData {
		lookup[row.Callsign] = row
	}

	newData, err := s.client.IFPSByDepartureAirport(ctx, airport)
	if err != nil {
		return err
	}

	for _, row := range newData {
		if flight, ok := lookup[row.Callsign]; ok {
			tsat := row.CDMData.TSAT
			ttot := row.CDMData.TTOT
			if len(tsat) > 4 {
				tsat = tsat[:4]
			}
			if len(ttot) > 4 {
				ttot = ttot[:4]
			}
			if flight.CdmStatus.String != row.CDMStatus ||
				flight.Aobt.String != row.AOBT ||
				flight.Eobt.String != row.EOBT ||
				flight.Ctot.String != row.CTOT ||
				flight.Tobt.String != row.TOBT ||
				flight.Tsat.String != tsat ||
				flight.Ttot.String != ttot {
				_, err = s.queries.UpdateCdmData(ctx, database.UpdateCdmDataParams{
					Session:   session,
					Callsign:  row.Callsign,
					Tobt:      pgtype.Text{Valid: true, String: row.TOBT},
					Tsat:      pgtype.Text{Valid: true, String: tsat},
					Ttot:      pgtype.Text{Valid: true, String: ttot},
					Ctot:      pgtype.Text{Valid: true, String: row.CTOT},
					Aobt:      pgtype.Text{Valid: true, String: row.AOBT},
					Eobt:      pgtype.Text{Valid: true, String: row.EOBT},
					CdmStatus: pgtype.Text{Valid: true, String: row.CDMStatus},
				})
				if err != nil {
					return err
				}

			}
		}
	}

	return nil
}
