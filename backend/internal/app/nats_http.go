package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"FlightStrips/internal/alb"
	"FlightStrips/internal/aman"
	amanWebAPI "FlightStrips/internal/aman/webapi"
	"FlightStrips/internal/cluster"
	appconfig "FlightStrips/internal/config"
	"FlightStrips/internal/efb"
	"FlightStrips/internal/euroscopebinary"
	"FlightStrips/internal/frontendbinary"
	"FlightStrips/internal/gsx"
	"FlightStrips/internal/httpresults"
	"FlightStrips/internal/metar"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/pilot"
	"FlightStrips/internal/server"
	"FlightStrips/internal/services"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/standstatus"
	"FlightStrips/internal/vatsim"
	pb "FlightStrips/pkg/events/cluster"
	modelvalues "FlightStrips/pkg/models"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (r *natsRuntime) Execute(ctx context.Context, req *pb.CommandRequest) *pb.CommandReply {
	return r.Route(ctx, req)
}

type natsHTTPReads struct{ r *natsRuntime }
type natsHTTPStrips struct{ natsHTTPReads }

func (s natsHTTPReads) List(ctx context.Context) ([]*models.Session, error) {
	entries, err := s.r.registry.ActiveSessions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*models.Session, 0, len(entries))
	for _, entry := range entries {
		a, err := s.r.projection.Read(sessionNATSRef(entry.Id))
		if err != nil {
			return nil, err
		}
		seed := a.Indexes[pb.EntityKind_SESSION][fmt.Sprint(entry.Id)].GetValue().GetSession()
		if seed == nil || seed.Tombstoned {
			continue
		}
		model := &models.Session{ID: seed.Id, Name: seed.Name, Airport: seed.Airport}
		for _, runway := range seed.Runways {
			if runway.Departure {
				model.ActiveRunways.DepartureRunways = append(model.ActiveRunways.DepartureRunways, runway.Name)
			}
			if runway.Arrival {
				model.ActiveRunways.ArrivalRunways = append(model.ActiveRunways.ArrivalRunways, runway.Name)
			}
		}
		for _, sid := range seed.AvailableSids {
			model.AvailableSids = append(model.AvailableSids, modelvalues.SidInfo{Name: sid.Name, Runway: sid.Runway})
		}
		out = append(out, model)
	}
	return out, nil
}
func (s natsHTTPReads) GetByNameAndAirport(ctx context.Context, name, airport string) (*models.Session, error) {
	values, err := s.List(ctx)
	for _, v := range values {
		if v.Name == name && v.Airport == airport {
			return v, nil
		}
	}
	return nil, err
}
func (s natsHTTPReads) GetByNames(ctx context.Context, name string) ([]*models.Session, error) {
	values, err := s.List(ctx)
	out := make([]*models.Session, 0)
	for _, v := range values {
		if v.Name == name {
			out = append(out, v)
		}
	}
	return out, err
}
func (s natsHTTPStrips) List(ctx context.Context, id int32) ([]*models.Strip, error) {
	a, err := s.r.projection.Read(sessionNATSRef(id))
	if err != nil {
		return nil, err
	}
	out := make([]*models.Strip, 0)
	for _, e := range a.EntitiesByKind(pb.EntityKind_STRIP) {
		out = append(out, services.CandidateModelStrip(e.Value.GetStrip(), id))
	}
	return out, nil
}
func (s natsHTTPStrips) GetByCallsign(ctx context.Context, id int32, callsign string) (*models.Strip, error) {
	a, err := s.r.projection.Read(sessionNATSRef(id))
	if err != nil {
		return nil, err
	}
	v := a.Indexes[pb.EntityKind_STRIP][callsign].GetValue().GetStrip()
	if v == nil {
		return nil, nil
	}
	return services.CandidateModelStrip(v, id), nil
}
func (s natsHTTPReads) ListAssignments(ctx context.Context, id int32) ([]*models.StandAssignment, error) {
	a, err := s.r.projection.Read(sessionNATSRef(id))
	if err != nil {
		return nil, err
	}
	out := make([]*models.StandAssignment, 0)
	for i, e := range a.EntitiesByKind(pb.EntityKind_STAND_ASSIGNMENT) {
		out = append(out, services.CandidateModelAssignment(e.Value.GetStandAssignment(), id, int64(i+1)))
	}
	return out, nil
}
func (s natsHTTPReads) ListBlocks(ctx context.Context, id int32) ([]*models.StandBlock, error) {
	a, err := s.r.projection.Read(sessionNATSRef(id))
	if err != nil {
		return nil, err
	}
	out := make([]*models.StandBlock, 0)
	for _, e := range a.EntitiesByKind(pb.EntityKind_STAND_BLOCK) {
		out = append(out, services.CandidateModelBlock(e.Value.GetStandBlock(), id))
	}
	return out, nil
}
func (s natsHTTPReads) snapshot(ctx context.Context) (vatsim.Snapshot, error) {
	_, page, _, err := s.r.source.CheckpointRevisionFor(ctx, globalNATSRef(), "vatsim", "network-data/v3")
	if err != nil {
		return vatsim.Snapshot{}, err
	}
	if page == nil {
		return vatsim.Snapshot{}, fmt.Errorf("accepted VATSIM feed unavailable")
	}
	return vatsim.SnapshotFromPage(page)
}
func (s natsHTTPReads) Snapshot() vatsim.Snapshot {
	v, err := s.snapshot(context.Background())
	if err != nil {
		v.LastRefreshError = err
	}
	return v
}
func (s natsHTTPReads) GetCallsignByCID(ctx context.Context, cid string) (string, bool, error) {
	v, err := s.snapshot(ctx)
	if err != nil {
		return "", false, err
	}
	if time.Since(v.Timestamp) > time.Minute {
		return "", false, fmt.Errorf("VATSIM feed stale")
	}
	flight, found := v.FlightByCID(cid)
	if !found || flight.State != vatsim.FlightStateOnline {
		return "", false, nil
	}
	return flight.Callsign, true, nil
}
func (s natsHTTPReads) VerifyPilotOwnsCallsign(ctx context.Context, cid, callsign string) (bool, error) {
	value, found, err := s.GetCallsignByCID(ctx, cid)
	return found && strings.EqualFold(value, callsign), err
}
func (s natsHTTPReads) GetFlightInfo(ctx context.Context, callsign string) (*pilot.FlightInfo, error) {
	flight, err := s.r.projection.FindFlight(ctx, callsign)
	if errors.Is(err, cluster.ErrFlightNotFound) {
		return nil, pilot.ErrFlightNotFound
	}
	if errors.Is(err, cluster.ErrAmbiguousFlight) {
		return nil, pilot.ErrAmbiguousCallsign
	}
	if err != nil {
		return nil, err
	}
	strip := flight.Strip
	cleared := strip.Bay != "NOT_CLEARED" && strip.Bay != "UNKNOWN"
	departure := strip.Departure == flight.Airport
	state := strip.PdcState
	if state == "" {
		state = "NONE"
	}
	if state == "REQUESTED_WITH_FAULTS" {
		state = "REQUESTED"
	}
	info := &pilot.FlightInfo{Callsign: strip.Callsign, Origin: strip.Departure, Destination: strip.Destination, IsDeparture: departure, Cleared: cleared, Eobt: httpClock(strip.Eobt), Tobt: httpClock(strip.Tobt), Ctot: httpClock(strip.Ctot), PushbackPoint: httpText(strip.ReleasePoint), PdcAvailable: departure && !cleared, PdcCanSubmit: departure && !cleared && pdc.WebPDCCanSubmit(strip.PdcState), PdcState: state, PdcRequiresPilotAction: state == "CLEARED", PdcRequestRemarks: httpText(strip.PdcRequestRemarks)}
	if flight.PDC != nil {
		info.PdcClearanceText = httpText(flight.PDC.ClearanceText)
		if flight.PDC.PilotAcknowledgedAt != nil {
			v := flight.PDC.PilotAcknowledgedAt.AsTime().Format(time.RFC3339)
			info.PdcAcknowledgedAt = &v
		}
	}
	return info, nil
}
func httpText(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
func httpClock(v *timestamppb.Timestamp) *string {
	if v == nil {
		return nil
	}
	value := v.AsTime().UTC().Format("1504")
	return &value
}
func (s natsHTTPReads) GetATIS(airport string, departure bool) *metar.ATIS {
	_, page, _, err := s.r.source.CheckpointRevisionFor(context.Background(), globalNATSRef(), "afv-atis", "feed")
	if err != nil || page.GetAtisFeed() == nil {
		return nil
	}
	for _, a := range page.GetAtisFeed().Airports {
		if a.Airport != airport {
			continue
		}
		v := a.Arrival
		if departure {
			v = a.Departure
		}
		if v == nil {
			return nil
		}
		return &metar.ATIS{Callsign: v.Callsign, Code: v.Code, Frequency: v.Frequency, Text: append([]string(nil), v.TextLines...), LastUpdated: v.LastUpdated.AsTime()}
	}
	return nil
}

func (s natsHTTPReads) ComputeDepartureFrequencyForStripContext(ctx context.Context, strip *models.Strip, id int32) (*string, error) {
	if strip == nil || strip.Sid == nil || strings.TrimSpace(*strip.Sid) == "" {
		return nil, nil
	}
	priority, err := appconfig.GetAirborneControllerPriority(strings.TrimSpace(*strip.Sid))
	if errors.Is(err, appconfig.ErrUnknownAirborneRoute) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_, presence, err := s.r.projection.ObservationSnapshot(id)
	if err != nil {
		return nil, err
	}
	controllers, err := (cluster.ControllerSector{Store: s.r.registry.Store}).OperationalControllers(ctx, id, presence, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	var coverage []appconfig.ControllerCoverage
	for _, controller := range controllers {
		if controller.Observer {
			continue
		}
		position, e := appconfig.GetPositionByName(controller.Position)
		if e != nil {
			position, e = appconfig.GetPositionBasedOnFrequency(controller.Position)
		}
		if e != nil {
			continue
		}
		value := appconfig.ControllerCoverage{Frequency: position.Frequency}
		if s.r.deadlines.Coverage != nil {
			value.CoveredFrequencies = s.r.deadlines.Coverage(controller.Callsign)
		}
		coverage = append(coverage, value)
	}
	return server.DepartureFrequency(priority, coverage), nil
}
func (r *natsRuntime) buildHTTP(cfg Config, deps Dependencies, auth shared.AuthenticationService, readiness appconfig.StandAssignmentReadiness, front frontendbinary.Handler, euro euroscopebinary.Handler) (http.Handler, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", r.metricsHTTP)
	api := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, req *http.Request) {
		result := healthResponse{Status: "ok", StandAssignment: satHealth{Enabled: readiness.Enabled, Ready: readiness.Ready, Status: "disabled"}, AMAN: aman.EvaluateTechnicalHealth(cfg.AMAN.Mode, aman.ComponentHealth{}, aman.ComponentHealth{}, aman.ComponentHealth{}, aman.ComponentHealth{}, aman.ComponentHealth{}, aman.ComponentHealth{})}
		if readiness.Enabled {
			result.StandAssignment = evaluateSATHealth(readiness, (natsHTTPReads{r}).Snapshot(), satStaleAfter(deps.VATSIMPollInterval), time.Now)
			if !result.StandAssignment.Ready {
				result.Status = "degraded"
			}
		}
		if r.aman != nil {
			state, err := r.projection.Read(airportNATSRef(string(r.terminal.Airport)))
			if err == nil {
				board, e := cluster.ReadAmanBoard(state)
				if e == nil && board.Airport != nil {
					components := map[string]aman.ComponentHealth{}
					for _, v := range board.Airport.GetHealth().GetComponents() {
						c := aman.ComponentHealth{Status: aman.HealthStatus(v.Status), Reason: v.Reason, AgeSeconds: v.AgeSeconds}
						if v.UpdatedAt != nil {
							at := v.UpdatedAt.AsTime()
							c.UpdatedAt = &at
						}
						components[v.Component] = c
					}
					result.AMAN = aman.EvaluateTechnicalHealth(cfg.AMAN.Mode, components["observation_source"], components["navigation"], components["weather"], components["repository"], components["predictor"], components["replay_validation"])
					result.AMAN.EffectiveMode = aman.EffectiveRolloutMode(board.Airport.EffectiveMode)
					result.AMAN.AuthorityAllowed = board.Airport.Authoritative && board.Airport.Health.GetReady()
					result.AMAN.Ready = board.Airport.Health.GetReady()
					result.AMAN.Status = aman.HealthStatus(board.Airport.Health.GetStatus())
					result.AMAN.BlockedReasons = board.Airport.Health.GetBlockedReasons()
				}
			}
			if !result.AMAN.Ready {
				result.Status = "degraded"
			}
		}
		writeNATSJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if err := r.ready(); err != nil {
			writeNATSJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable", "reason": err.Error()})
			return
		}
		writeNATSJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.Handle("/frontEndEvents", front)
	mux.Handle("/euroscopeEvents", euro)
	if cfg.EnableALB {
		hub := alb.NewHub()
		go hub.Run()
		mux.HandleFunc("/albEvents", hub.Upgrade)
	}
	reads := natsHTTPReads{r}
	strips := natsHTTPStrips{reads}
	standReads := services.StandReadCandidate{State: r.stands, HoldDuration: cfg.StandAssignmentHoldDuration, BlockExtension: cfg.StandAssignmentBlockExtension}
	standstatus.NewWebAPI(standstatus.WebAPIConfig{Auth: auth, Sessions: reads, Assignments: reads, Strips: strips, Feed: reads, Enabled: readiness.Enabled, Ready: readiness.Ready, Reason: readiness.Reason, StaleAfter: satStaleAfter(deps.VATSIMPollInterval), Diagnostics: standAssignmentDiagnostics(), Previewer: standReads}).RegisterRoutes(api)
	if cfg.EnableCDMConfigStore {
		cdmAPI, err := services.NewCdmCandidateWebAPI(auth, r.cdm)
		if err != nil {
			return nil, err
		}
		cdmAPI.RegisterRoutes(api)
	}
	if cfg.EnablePDC {
		pdc.NewCandidateWebAPI(auth, reads, isLiveEnvironment(cfg.Environment), r.projection, r, r.projection).RegisterRoutes(api)
	}
	if cfg.EnablePilotAPI {
		pilot.NewWebAPI(auth, reads, reads, isLiveEnvironment(cfg.Environment)).RegisterRoutes(api)
	}
	if cfg.EnableEFB {
		var availability efb.CandidateStandAvailability
		if cfg.EnableStandAssignment {
			availability = standReads
		}
		efb.NewCandidateWebAPI(efb.WebAPIConfig{Auth: auth, Callsigns: reads, CDMReady: cfg.EnableCDMConfigStore, PDCReady: cfg.EnablePDC, Live: isLiveEnvironment(cfg.Environment), ATIS: reads, Departures: reads, Terminal: r.terminal, Navigation: r.navigation}, r.projection, r, r.projection, availability).RegisterRoutes(api)
	}
	if cfg.EnableGSXStandFeed {
		sceneries, err := loadGSXSceneries(true)
		if err != nil {
			return nil, err
		}
		gsx.NewWebAPI(reads, strips, sceneries, true).RegisterRoutes(api)
	}
	if cfg.EnableECFMPAPI {
		r.registerECFMP(api)
	}
	if cfg.AMAN.Mode != aman.ModeDisabled {
		amanWebAPI.New(auth, r.aman).WithNavigation(r.aman, r.aman).RegisterRoutes(api)
	}
	(httpresults.Query{Auth: auth, Outcomes: r.projection}).RegisterRoutes(api)
	mux.Handle("/api/", server.APIMiddleware(http.StripPrefix("/api", api)))
	if cfg.EnableHTTPTracing {
		return otelhttp.NewHandler(mux, "http.server"), nil
	}
	return mux, nil
}
func writeNATSJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
