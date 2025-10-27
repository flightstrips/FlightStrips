package server

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/shared"
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	dbPool       *pgxpool.Pool
	euroscopeHub shared.EuroscopeHub
	frontendHub  shared.FrontendHub
}

func NewServer(dbPool *pgxpool.Pool, euroscopeHub shared.EuroscopeHub, frontendHub shared.FrontendHub) *Server {
	server := Server{
		dbPool:       dbPool,
		euroscopeHub: euroscopeHub,
		frontendHub:  frontendHub,
	}

	go server.monitorSessions()

	return &server
}

func (s *Server) GetDatabasePool() *pgxpool.Pool {
	return s.dbPool
}

func (s *Server) GetEuroscopeHub() shared.EuroscopeHub {
	return s.euroscopeHub
}

func (s *Server) GetFrontendHub() shared.FrontendHub {
	return s.frontendHub
}

func (s *Server) GetOrCreateSession(airport string, name string) (shared.Session, error) {
	db := database.New(s.dbPool)

	arg := database.GetSessionParams{Name: name, Airport: airport}
	session, err := db.GetSession(context.Background(), arg)

	if err == nil {
		return shared.Session{Name: session.Name, Airport: session.Airport, Id: session.ID}, nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		log.Println("Creating session:", name, "for airport:", airport)
		insertArg := database.InsertSessionParams{Name: name, Airport: airport}
		id, err := db.InsertSession(context.Background(), insertArg)

		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				session, err = db.GetSession(context.Background(), arg)
				if err != nil {
					return shared.Session{Name: name, Airport: airport, Id: id}, err
				}
			}

			return shared.Session{}, err

		}

		return shared.Session{Name: name, Airport: airport, Id: id}, err
	}

	return shared.Session{}, nil
}

func (s *Server) monitorSessions() {
	for {
		expired := time.Now().Add(-time.Minute * 5).UTC()
		db := database.New(s.dbPool)

		sessions, err := db.GetExpiredSessions(context.Background(), pgtype.Timestamp{Time: expired, Valid: true})

		if err != nil {
			log.Println("Failed to get expired sessions:", err)
		}

		for _, session := range sessions {
			log.Println("Removing expired session:", session)
			count, err := db.DeleteSession(context.Background(), session)
			if err != nil {
				log.Println("Failed to remove expired session:", session, err)
			}

			if count != 1 {
				log.Println("Failed to remove expired session (no changes):", session, err)
			}
		}

		time.Sleep(time.Minute)
	}
}
