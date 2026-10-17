package server

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/services"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise SID selection, live coverage resolution, coordination creation and
// command emission together, rather than calling CreateCoordinationTransfer directly.
func TestAutoTransferAirborneStripRadioCoverage(t *testing.T) {
	for _, tt := range []struct {
		name                 string
		controllers          []*models.Controller
		coverage             routeTransceiverStub
		wantPosition         string
		wantCallsign         string
		afterCoverageRemoval string
		ownerPrimary         string
	}{
		{
			name:         "separately primed DEP",
			controllers:  []*models.Controller{{Callsign: "EKCH_K_DEP", Position: "124.980"}},
			wantPosition: "124.980", wantCallsign: "EKCH_K_DEP",
		},
		{
			name:         "cross coupled DEP wins over APP fallback",
			controllers:  []*models.Controller{{Callsign: "EKCH_O_APP", Position: "118.455"}, {Callsign: "EKCH_F_APP", Position: "120.205"}},
			coverage:     routeTransceiverStub{"EKCH_F_APP": {"124980000"}},
			wantPosition: "120.205", wantCallsign: "EKCH_F_APP",
			afterCoverageRemoval: "EKCH_O_APP",
		},
		{
			name:         "APP fallback when DEP is not carried",
			controllers:  []*models.Controller{{Callsign: "EKCH_F_APP", Position: "120.205"}, {Callsign: "EKCH_O_APP", Position: "118.455"}},
			wantPosition: "118.455", wantCallsign: "EKCH_O_APP",
		},
		{
			name:         "observer primary cannot displace DEP carrier",
			controllers:  []*models.Controller{{Callsign: "EKCH_K_DEP", Position: "124.980", Observer: true}, {Callsign: "EKCH_F_APP", Position: "120.205"}},
			coverage:     routeTransceiverStub{"EKCH_F_APP": {"124.980"}},
			wantPosition: "120.205", wantCallsign: "EKCH_F_APP",
		},
		{
			name:         "CTR fallback when DEP and APP are absent",
			controllers:  []*models.Controller{{Callsign: "EKDK_I_CTR", Position: "121.380"}},
			wantPosition: "121.380", wantCallsign: "EKDK_I_CTR",
		},
		{
			name:         "carrying callsign retained when observer shares primary",
			controllers:  []*models.Controller{{Callsign: "EKCH_F_APP", Position: "120.205"}, {Callsign: "EKCH_F_APP_OBS", Position: "120.205", Observer: true}},
			coverage:     routeTransceiverStub{"EKCH_F_APP": {"124.980"}},
			wantPosition: "120.205", wantCallsign: "EKCH_F_APP",
		},
		{
			name:         "observer coverage cannot displace APP fallback",
			controllers:  []*models.Controller{{Callsign: "EKCH_F_APP", Position: "120.205", Observer: true}, {Callsign: "EKCH_O_APP", Position: "118.455"}},
			coverage:     routeTransceiverStub{"EKCH_F_APP": {"124.980"}},
			wantPosition: "118.455", wantCallsign: "EKCH_O_APP",
		},
		{
			name:        "non operational callsign ignored",
			controllers: []*models.Controller{{Callsign: "TEST_APP", Position: "124.980"}},
		},
		{
			name:        "no carrier or fallback",
			controllers: []*models.Controller{{Callsign: "EKCH_F_APP", Position: "120.205"}},
		},
		{
			name:     "owner carries DEP so no self handover",
			coverage: routeTransceiverStub{"EKCH_A_TWR": {"124.980"}},
		},
		{
			name:         "normalized owner identity prevents self handover",
			coverage:     routeTransceiverStub{"EKCH_A_TWR": {"124.980"}},
			ownerPrimary: "118105000",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			cid, owner, sid := "123456", "118.105", "NEXEN2A"
			ownerPrimary := tt.ownerPrimary
			if ownerPrimary == "" {
				ownerPrimary = owner
			}
			controllers := append([]*models.Controller{{Callsign: "EKCH_A_TWR", Position: ownerPrimary, Cid: &cid}}, tt.controllers...)
			controllerRepo := &testutil.MockControllerRepository{
				ListFn:          func(context.Context, int32) ([]*models.Controller, error) { return controllers, nil },
				ListBySessionFn: func(context.Context, int32) ([]*models.Controller, error) { return controllers, nil },
			}
			radio := &Server{controllerRepo: controllerRepo, frequencyProviders: []TransceiverLookup{tt.coverage}}
			strip := &models.Strip{ID: 1, Callsign: "SAS880", Bay: shared.BAY_AIRBORNE, Owner: &owner, Sid: &sid}
			stripRepo := &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil }}
			var created []*models.Coordination
			coordRepo := &testutil.MockCoordinationRepository{
				GetByStripIDFn: func(context.Context, int32, int32) (*models.Coordination, error) { return nil, pgx.ErrNoRows },
				CreateFn:       func(_ context.Context, c *models.Coordination) error { created = append(created, c); return nil },
			}
			frontend, euroscope := &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{}
			svc := services.NewStripService(stripRepo)
			svc.SetControllerRepo(controllerRepo)
			svc.SetCoordinationRepo(coordRepo)
			svc.SetFrontendHub(frontend)
			svc.SetEuroscopeHub(euroscope)
			svc.SetRouteRecalculator(radio)
			require.NoError(t, svc.AutoTransferAirborneStrip(ctx, 437, strip.Callsign))
			if tt.wantPosition == "" {
				assert.Empty(t, created)
				assert.Empty(t, frontend.CoordinationTransfers)
				assert.Empty(t, euroscope.CoordinationHandovers)
				return
			}
			require.Len(t, created, 1)
			assert.Equal(t, tt.wantPosition, created[0].ToPosition)
			assert.Equal(t, owner, created[0].FromPosition)
			require.Len(t, frontend.CoordinationTransfers, 1)
			assert.Equal(t, tt.wantPosition, frontend.CoordinationTransfers[0].To)
			require.Len(t, euroscope.CoordinationHandovers, 1)
			command := euroscope.CoordinationHandovers[0]
			assert.Equal(t, int32(437), command.Session)
			assert.Equal(t, cid, command.Cid)
			assert.Equal(t, strip.Callsign, command.Callsign)
			assert.Equal(t, tt.wantCallsign, command.TargetCallsign)
			if tt.afterCoverageRemoval != "" {
				// The same service must consult the current provider snapshot on
				// the next transfer instead of retaining its previous DEP carrier.
				delete(tt.coverage, "EKCH_F_APP")
				require.NoError(t, svc.AutoTransferAirborneStrip(ctx, 437, strip.Callsign))
				require.Len(t, created, 2)
				assert.Equal(t, "118.455", created[1].ToPosition)
				require.Len(t, euroscope.CoordinationHandovers, 2)
				assert.Equal(t, tt.afterCoverageRemoval, euroscope.CoordinationHandovers[1].TargetCallsign)
			}
		})
	}
}
