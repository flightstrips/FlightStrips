package cdm

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/helpers"
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	client      *Client
	queries     *database.Queries
	frontendHub shared.FrontendHub
}

func NewCdmService(client *Client, dbPool *pgxpool.Pool) *Service {
	return &Service{client: client, queries: database.New(dbPool)}
}

func (s *Service) SetFrontendHub(frontendHub shared.FrontendHub) {
	s.frontendHub = frontendHub
}

func (s *Service) SetReady(ctx context.Context, session int32, callsign string) error {
	if !s.client.isValid {
		return nil
	}
	cdmData, err := s.queries.GetCdmDataForCallsign(ctx, database.GetCdmDataForCallsignParams{Callsign: callsign, Session: session})
	if err != nil {
		return err
	}

	if cdmData.CdmStatus != nil && *cdmData.CdmStatus == "REA" {
		return nil
	}

	if err := s.client.IFPSSetCDMStatus(ctx, callsign, "REA"); err != nil {
		return err
	}

	rea := "REA"
	rows, err := s.queries.SetCdmStatus(ctx, database.SetCdmStatusParams{
		CdmStatus: &rea, Callsign: callsign, Session: session,
	})
	if err != nil {
		return err
	}

	if rows != 1 {
		return fmt.Errorf("failed to update CDM status for %s session %d", callsign, session)
	}

	s.frontendHub.SendCdmWait(session, callsign)

	return nil
}

func (s *Service) RequestBetterTobt(ctx context.Context, session int32, callsign string) error {
	if !s.client.isValid {
		return nil
	}
	now := time.Now()
	// format hhmm
	format := now.Format("1504")
	status := "REQTOBT/" + format + "/ATC"

	cdmData, err := s.queries.GetCdmDataForCallsign(ctx, database.GetCdmDataForCallsignParams{Callsign: callsign, Session: session})
	if err != nil {
		return err
	}

	if cdmData.CdmStatus != nil && *cdmData.CdmStatus == status {
		return nil
	}

	err = s.client.IFPSSetCDMStatus(ctx, callsign, status)
	if err != nil {
		return err
	}

	rows, err := s.queries.SetCdmStatus(ctx, database.SetCdmStatusParams{
		CdmStatus: &status, Callsign: callsign, Session: session,
	})
	if err != nil {
		return err
	}

	if rows != 1 {
		return fmt.Errorf("failed to update CDM status for %s session %d", callsign, session)
	}

	s.frontendHub.SendCdmWait(session, callsign)

	return nil
}

func (s *Service) Start(ctx context.Context) {
	if !s.client.isValid {
		fmt.Println("CDM client is not valid, CDM data will not be synced")
		return
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		sessions, err := s.queries.GetSessions(ctx)
		if err != nil {
			continue
		}
		for _, session := range sessions {
			if session.Name != "LIVE" {
				continue
			}

			fmt.Println("Syncing CDM data for session", session.Name, "id", session.ID, "airport", session.Airport)

			err = s.syncCdmData(ctx, session)
			if err != nil {
				fmt.Println("Failed to sync CDM data:", err)
			}
		}
	}
}

func (s *Service) syncCdmData(ctx context.Context, session database.Session) error {
	if !s.client.isValid {
		return nil
	}

	airport := session.Airport

	currentData, err := s.queries.GetCdmData(ctx, session.ID)
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
			if helpers.ValueOrDefault(flight.CdmStatus) != row.CDMStatus ||
				helpers.ValueOrDefault(flight.Aobt) != row.AOBT ||
				helpers.ValueOrDefault(flight.Eobt) != row.EOBT ||
				helpers.ValueOrDefault(flight.Ctot) != row.CTOT ||
				helpers.ValueOrDefault(flight.Tobt) != row.TOBT ||
				helpers.ValueOrDefault(flight.Tsat) != tsat ||
				helpers.ValueOrDefault(flight.Ttot) != ttot {
				_, err = s.queries.UpdateCdmData(ctx, database.UpdateCdmDataParams{
					Session:   session.ID,
					Callsign:  row.Callsign,
					Tobt:      &row.TOBT,
					Tsat:      &tsat,
					Ttot:      &ttot,
					Ctot:      &row.CTOT,
					Aobt:      &row.AOBT,
					Eobt:      &row.EOBT,
					CdmStatus: &row.CDMStatus,
				})
				if err != nil {
					return err
				}

				s.frontendHub.SendCdmUpdate(session.ID, row.Callsign, row.EOBT, row.TOBT, tsat, row.CTOT)
			}
		}
	}

	return nil
}
