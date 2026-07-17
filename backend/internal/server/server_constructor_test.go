package server

import (
	"FlightStrips/internal/testutil"
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

type constructorCdmService struct{}

func (constructorCdmService) TriggerRecalculate(context.Context, int32, string) {}
func (constructorCdmService) SyncAirportLvoFromRunwayStatus(context.Context, string, map[string]string) {
}
func (constructorCdmService) HandleReadyRequest(context.Context, int32, string, string, string) error {
	return nil
}
func (constructorCdmService) HandleEobtUpdate(context.Context, int32, string, string, string, string) error {
	return nil
}
func (constructorCdmService) HandleTobtUpdate(context.Context, int32, string, string, string, string) error {
	return nil
}
func (constructorCdmService) HandleClxTobtUpdate(context.Context, int32, string, string, string, string) error {
	return nil
}
func (constructorCdmService) HandleDeiceUpdate(context.Context, int32, string, string) error {
	return nil
}
func (constructorCdmService) HandleAsrtToggle(context.Context, int32, string, string) error {
	return nil
}
func (constructorCdmService) HandleTsacUpdate(context.Context, int32, string, string) error {
	return nil
}
func (constructorCdmService) HandleManualCtot(context.Context, int32, string, string) error {
	return nil
}
func (constructorCdmService) HandleCtotRemove(context.Context, int32, string) error { return nil }
func (constructorCdmService) HandleApproveReqTobt(context.Context, int32, string, string, string) error {
	return nil
}
func (constructorCdmService) SyncAsatForGroundState(context.Context, int32, string, string) error {
	return nil
}
func (constructorCdmService) RequestBetterTobt(context.Context, int32, string) error { return nil }
func (constructorCdmService) SetSessionCdmMaster(context.Context, int32, bool) error { return nil }

func validServerDependencies() Dependencies {
	return Dependencies{
		DBPool:           &pgxpool.Pool{},
		Euroscope:        &testutil.MockEuroscopeHub{},
		Frontend:         &testutil.MockFrontendHub{},
		CDM:              constructorCdmService{},
		Strips:           &testutil.MockStripRepository{},
		Controllers:      &testutil.MockControllerRepository{},
		Sessions:         &testutil.MockSessionRepository{},
		Sectors:          &testutil.MockSectorOwnerRepository{},
		Coordinations:    &testutil.MockCoordinationRepository{},
		TacticalStrips:   &testutil.MockTacticalStripRepository{},
		StandAssignments: nil,
	}
}

func TestNewServerRejectsMissingRequiredDependencies(t *testing.T) {
	tests := []struct {
		name   string
		remove func(*Dependencies)
		want   string
	}{
		{"database pool", func(d *Dependencies) { d.DBPool = nil }, "server requires database pool"},
		{"EuroScope hub", func(d *Dependencies) { d.Euroscope = nil }, "server requires EuroScope hub"},
		{"frontend hub", func(d *Dependencies) { d.Frontend = nil }, "server requires frontend hub"},
		{"CDM service", func(d *Dependencies) { d.CDM = nil }, "server requires CDM service"},
		{"strip repository", func(d *Dependencies) { d.Strips = nil }, "server requires strip repository"},
		{"controller repository", func(d *Dependencies) { d.Controllers = nil }, "server requires controller repository"},
		{"session repository", func(d *Dependencies) { d.Sessions = nil }, "server requires session repository"},
		{"sector repository", func(d *Dependencies) { d.Sectors = nil }, "server requires sector repository"},
		{"coordination repository", func(d *Dependencies) { d.Coordinations = nil }, "server requires coordination repository"},
		{"tactical strips", func(d *Dependencies) { d.TacticalStrips = nil }, "server requires tactical strip repository"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := validServerDependencies()
			test.remove(&deps)
			_, err := NewServer(deps)
			require.EqualError(t, err, test.want)
		})
	}
}

func TestNewServerAcceptsNoFrequencyProviders(t *testing.T) {
	server, err := NewServer(validServerDependencies())
	require.NoError(t, err)
	require.Empty(t, server.frequencyProviders)
}

func TestNewServerRejectsNilFrequencyProvider(t *testing.T) {
	deps := validServerDependencies()
	deps.FrequencyProviders = []TransceiverLookup{nil}
	_, err := NewServer(deps)
	require.EqualError(t, err, "server frequency provider 0 is nil")
}
