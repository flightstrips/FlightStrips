package shared

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

type ServerInjectable interface {
	GetServer() Server
	SetServer(server Server)
}

type Session struct {
	Id      int32
	Name    string
	Airport string
}

type Server interface {
	GetDatabasePool() *pgxpool.Pool
	GetEuroscopeHub() EuroscopeHub
	GetFrontendHub() FrontendHub
	GetOrCreateSession(airport string, name string) (Session, error)

	// TODO move to another service
	UpdateSectors(sessionId int32) error
	UpdateRouteForStrip(callsign string, sessionId int32, sendUpdate bool) error
	UpdateRoutesForSession(sessionId int32, sendUpdate bool) error
}

type ConnectedUser struct {
	Cid      string
	Session  int32
	Position string
	Callsign string
	Airport  string
}
