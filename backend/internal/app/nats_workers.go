package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata/airacnet"
	"FlightStrips/internal/aman/predictor"
	"FlightStrips/internal/aman/predictor/openmeteo"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/amancandidate"
	"FlightStrips/internal/cdm"
	"FlightStrips/internal/cluster"
	appconfig "FlightStrips/internal/config"
	"FlightStrips/internal/ecfmp"
	"FlightStrips/internal/metar"
	"FlightStrips/internal/navigationcandidate"
	"FlightStrips/internal/services"
	"FlightStrips/internal/vatsim"
	"FlightStrips/internal/vatsimcandidate"
	pb "FlightStrips/pkg/events/cluster"
)

func buildNATSCdm(cfg Config, deps Dependencies, writer cluster.Writer, source cluster.NavigationWeather) (*services.CdmCandidate, error) {
	opts := []cdm.Option{cdm.WithAPIKey(cfg.CDMKey), cdm.WithAirportMasterCacheTTL(0)}
	if deps.NATS.HTTPClient != nil {
		opts = append(opts, cdm.WithHTTPClient(deps.NATS.HTTPClient))
	}
	if deps.NATS.CDMBaseURL != "" {
		opts = append(opts, cdm.WithBaseURL(deps.NATS.CDMBaseURL))
	}
	client := cdm.NewClient(opts...)
	defaults := appconfig.GetCdmConfig()
	resolve := func(uri string) string {
		if !cfg.EnableCDMConfigStore {
			return ""
		}
		if uri == "" || strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
			return uri
		}
		return filepath.Join(cfg.CDMConfigDir, uri)
	}
	store := cdm.NewCdmConfigStore(resolve(defaults.RateUri), resolve(defaults.SidIntervalUri), resolve(defaults.TaxizonesUri), cfg.CDMConfigRefreshInterval, cdm.CdmConfigDefaults{Rate: defaults.Rate, RateLvo: defaults.RateLvo, TaxiMinutes: defaults.DefaultTaxiTime}, deps.NATS.HTTPClient)
	platforms := make([]cdm.CdmDeicePlatformConfig, 0, len(defaults.Deice.Platform))
	for _, p := range defaults.Deice.Platform {
		platforms = append(platforms, cdm.CdmDeicePlatformConfig{Name: p.Name, Time: p.Time})
	}
	store.SeedAirportConfig("EKCH", defaults.Rate, defaults.RateLvo, cdm.CdmDeiceConfig{Light: defaults.Deice.Light, Medium: defaults.Deice.Medium, Heavy: defaults.Deice.Heavy, Super: defaults.Deice.Super, Platform: platforms})
	external := cluster.ExternalCallWorker{Writer: writer, ResultWriter: writer}
	config := cluster.CdmConfigCandidateWorker{State: source, Worker: external, Fetch: store.FetchAirportCandidate}
	reads, writes := cluster.ViffReadAdapter{State: source, Worker: external}, cluster.ViffWriteAdapter{Worker: external}
	if cfg.CDMKey != "" {
		if cfg.EnableCDMConfigStore {
			store.SetCdmClient(client)
		}
		reads.Client = client
		writes.Client = client
	}
	return services.NewCdmCandidate(writer, config, reads, writes)
}
func (r *natsRuntime) airports(configured []string) ([]string, error) {
	entities, err := r.projection.ReadEntities(globalNATSRef(), pb.EntityKind_SESSION_REGISTRY)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, icao := range configured {
		set[icao] = true
	}
	for _, e := range entities {
		s := e.Value.GetSessionRegistry()
		if s.State != pb.SessionRegistry_DELETED {
			set[s.Airport] = true
		}
	}
	out := make([]string, 0, len(set))
	for icao := range set {
		out = append(out, icao)
	}
	sort.Strings(out)
	return out, nil
}
func slot(now time.Time, interval time.Duration) time.Time {
	if interval <= 0 {
		interval = time.Minute
	}
	return now.UTC().Truncate(interval)
}
func (r *natsRuntime) assembleProviders(cfg Config, deps Dependencies, transceivers *cluster.TransceiverSource) error {
	writer := r.router.Writer
	external := cluster.ExternalCallWorker{Writer: writer, ResultWriter: writer}
	vatsimPoll := deps.VATSIMPollInterval
	if vatsimPoll <= 0 {
		vatsimPoll = 15 * time.Second
	}
	vatsimFeed := vatsimcandidate.Fetch{Cache: vatsim.NewCache(deps.VATSIMStatusURL, vatsimPoll, deps.NATS.HTTPClient), State: r.source, Worker: external}
	transceiverPoll := deps.TransceiversInterval
	if transceiverPoll <= 0 {
		transceiverPoll = 30 * time.Second
	}
	transceiverFeed, err := cluster.NewTransceiverFeed(r.source, vatsim.NewTransceiverProvider(deps.TransceiversURL, transceiverPoll, deps.NATS.HTTPClient))
	if err != nil {
		return err
	}
	ecfmpOpts := []ecfmp.Option{ecfmp.WithBaseURL(cfg.ECFMPBaseURL)}
	if deps.NATS.HTTPClient != nil {
		ecfmpOpts = append(ecfmpOpts, ecfmp.WithHTTPClient(deps.NATS.HTTPClient))
	}
	ecfmpFeed := ecfmp.CandidateFetch{Client: ecfmp.NewClient(ecfmpOpts...), State: r.source, Worker: external}
	metarFeed := metar.Candidate{Provider: metar.NewCandidateProvider(deps.NATS.HTTPClient, deps.NATS.MetarBaseURL, deps.NATS.ATISURL), State: r.source, AirportWorker: external, GlobalWorker: external, Session: cluster.AtisSessionAdapter{Writer: writer}}
	var globalEpoch uint64
	var nextProviderSweep time.Time
	r.supervisors = append(r.supervisors, func(ctx context.Context) error {
		return periodic(ctx, time.Second, func(ctx context.Context, now time.Time) error {
			if !r.owner.CanWrite(globalNATSRef()) {
				globalEpoch = 0
				return nil
			}
			owner, err := r.projection.ReadOwner(globalNATSRef())
			if err != nil {
				return err
			}
			if globalEpoch != owner.GetEpoch() {
				if err = external.Resume(ctx, globalNATSRef()); err != nil {
					return err
				}
				nextProviderSweep = time.Time{}
				globalEpoch = owner.GetEpoch()
			}
			if !now.Before(nextProviderSweep) {
				if _, err = r.source.PruneProviderPages(ctx, globalNATSRef()); err != nil {
					return err
				}
				if _, err = r.projection.Snapshots.Prune(ctx, globalNATSRef()); err != nil {
					return err
				}
				nextProviderSweep = now.Add(time.Minute)
			}
			var failures []error
			if cfg.EnableVATSIM {
				_, e := vatsimFeed.Global(ctx, slot(now, vatsimPoll))
				failures = append(failures, e)
			}
			if cfg.EnableTransceivers {
				_, e := transceiverFeed.Refresh(ctx, now)
				failures = append(failures, e)
			}
			if cfg.EnableECFMP {
				id, _ := cluster.ProviderEventCommandID("ecfmp-feed", "ecfmp", slot(now, time.Minute).Format(time.RFC3339Nano))
				_, e := ecfmpFeed.FetchGlobal(ctx, id)
				failures = append(failures, e)
			}
			if cfg.EnableMetar {
				_, e := metarFeed.FetchAtisFeed(ctx, slot(now, time.Minute))
				failures = append(failures, e)
			}
			return errors.Join(failures...)
		})
	})
	var airac *cluster.AiracCandidateWorker
	if cfg.Navigation.Enabled() {
		terminalConfig, e := terminal.LoadFile(cfg.Navigation.TerminalGeometryPath)
		if e != nil {
			return e
		}
		r.terminal = terminalConfig
		volume, e := terminal.LoadTMAVolume(terminal.DefaultEKCHTMAVolumePath)
		if e != nil {
			return e
		}
		adapter, e := airacnet.New(airacnet.Config{BaseURL: deps.NATS.AIRACBaseURL, HTTPClient: deps.NATS.HTTPClient, Checkpoints: airacnet.NewMemoryCheckpoints()})
		if e != nil {
			return e
		}
		source := navigationcandidate.FromAiracNet(adapter, terminalConfig)
		airac = &cluster.AiracCandidateWorker{State: r.source, Worker: external, Fetch: func(ctx context.Context, icao string, prior *pb.ProviderCheckpoint, previous *pb.ProviderPage) (*pb.AiracPage, *pb.ProviderCheckpoint, error) {
			page, cp, err := source.Fetch(ctx, icao, prior, previous)
			if err == nil {
				for _, fragment := range page.Fragments {
					if t := fragment.GetTerminalFragment(); t != nil {
						t.OperationalPolicy = amancandidate.TerminalPolicy(terminalConfig, volume)
					}
				}
			}
			return page, cp, err
		}}
		if cfg.AMAN.Mode != aman.ModeDisabled {
			// Each evaluation reads accepted pages or acquires them through the airport
			// intent and global quota protocol, never the adapter's local quota cache.
			r.aman, e = amancandidate.New(amancandidate.Options{Source: r.source, State: cluster.AmanAdapter{Writer: writer}, Sessions: r.registry.Store, Projection: r.projection, RouteWorker: external, RouteResolver: adapter, Terminal: terminalConfig, AircraftEngines: appconfig.GetAircraftEngineReference(), Mode: cfg.AMAN.Mode, SourceMode: cfg.AMAN.SourceMode, VatsimStaleAfter: satStaleAfter(vatsimPoll), HoldingEATEnabled: cfg.AMAN.EnableHoldingEATWriteback, WindForAirport: func(airport string, at time.Time) predictor.WindProfileReader {
				// Provider observations use the accepted evaluation clock. A wall
				// clock sampled after HTTP would appear to come from the future to
				// the operational policy evaluating this fixed accepted tick.
				wind := openmeteo.Candidate{Provider: openmeteo.New(openmeteo.Config{BaseURL: deps.NATS.OpenMeteoBaseURL, Client: deps.NATS.HTTPClient, Now: func() time.Time { return at }}), State: r.source, Worker: external}
				return natsWind{candidate: wind, runtime: r, airport: airport, at: at}
			}})
			if e != nil {
				return e
			}
		}
		r.navigation = r.aman
		if r.navigation == nil {
			r.navigation, e = amancandidate.New(amancandidate.Options{Source: r.source, State: cluster.AmanAdapter{Writer: writer}, Sessions: r.registry.Store, Projection: r.projection, RouteWorker: external, RouteResolver: adapter, Terminal: terminalConfig, AircraftEngines: appconfig.GetAircraftEngineReference(), Mode: aman.ModeDisabled})
			if e != nil {
				return e
			}
		}
	} else if cfg.AMAN.Mode != aman.ModeDisabled {
		return fmt.Errorf("enabled AMAN requires navigation source")
	}
	epochs := map[string]uint64{}
	airportSweeps := map[string]time.Time{}
	r.supervisors = append(r.supervisors, func(ctx context.Context) error {
		return periodic(ctx, time.Second, func(ctx context.Context, now time.Time) error {
			airports, err := r.airports(append(cfg.NATS.Airports, cfg.AMAN.EnabledAirports...))
			if err != nil {
				return err
			}
			var failures []error
			for _, icao := range airports {
				ref := airportNATSRef(icao)
				_ = r.owner.Track(ref)
				if !r.owner.CanWrite(ref) {
					delete(epochs, icao)
					delete(r.vatsimPasses, icao)
					continue
				}
				owner, e := r.projection.ReadOwner(ref)
				if e != nil {
					failures = append(failures, e)
					continue
				}
				if epochs[icao] != owner.GetEpoch() {
					if e = external.Resume(ctx, ref); e != nil {
						failures = append(failures, e)
						continue
					}
					if airac != nil {
						if e = airac.Resume(ctx, icao); e != nil {
							failures = append(failures, e)
							continue
						}
					}
					if r.aman != nil {
						if e = r.aman.Resume(ctx, icao); e != nil {
							failures = append(failures, e)
							continue
						}
					}
					delete(airportSweeps, icao)
					epochs[icao] = owner.GetEpoch()
				}
				if !now.Before(airportSweeps[icao]) {
					if _, e = r.source.PruneProviderPages(ctx, ref); e != nil {
						failures = append(failures, e)
						continue
					}
					if _, e = r.projection.Snapshots.Prune(ctx, ref); e != nil {
						failures = append(failures, e)
						continue
					}
					airportSweeps[icao] = now.Add(time.Minute)
				}
				configPage, _, configErr := r.cdm.Config.Read(ctx, icao)
				if cfg.EnableCDMConfigStore || configErr != nil || configPage == nil {
					_, e = r.cdm.Config.Refresh(ctx, icao, slot(now, cfg.CDMConfigRefreshInterval))
					failures = append(failures, e)
				}
				if r.cdm.Reads.Client != nil {
					_, e = r.cdm.Reads.Masters(ctx, icao, slot(now, 15*time.Second))
					failures = append(failures, e)
					e = r.cdm.ReconcileMaster(ctx, icao)
					failures = append(failures, e)
				}
				if cfg.EnableMetar {
					_, e = metarFeed.FetchMetar(ctx, icao, slot(now, time.Minute), 10*time.Minute, func(ctx context.Context, id string) (bool, error) {
						return r.reserveQuota(ctx, id, "metar", slot(now, time.Minute), 500)
					})
					failures = append(failures, e)
				}
				if airac != nil {
					_, e = airac.Import(ctx, icao, slot(now, 6*time.Hour))
					failures = append(failures, e)
				}
				if r.aman != nil && containsAirport(cfg.AMAN.EnabledAirports, icao) {
					failures = append(failures, r.reconcileAMAN(ctx, icao, now, cfg.AMAN))
				}
			}
			return errors.Join(failures...)
		})
	})
	sector, err := cluster.NewTransceiverSectorReconciler(transceivers, writer, r.deadlines.PlanTransceiverSectors)
	if err != nil {
		return err
	}
	election := cluster.MasterElection{Projection: r.projection, Router: r, Lease: r.owner}
	original := r.work.EuroScope
	r.work.EuroScope = func(ctx context.Context, id int32) error {
		var failures []error
		_, err := election.Reconcile(ctx, id)
		failures = append(failures, err)
		failures = append(failures, original(ctx, id))
		if cfg.EnableTransceivers {
			failures = append(failures, natsReply(sector.Reconcile(ctx, id)))
		}
		state, err := r.projection.Read(sessionNATSRef(id))
		if err != nil {
			return err
		}
		airport := state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(id)].Value.GetSession().Airport
		if cfg.EnableVATSIM && r.work.Departure == nil {
			failures = append(failures, natsReply((cluster.VatsimSessionAdapter{Source: r.source, Writer: writer}).Reconcile(ctx, id)))
		}
		if cfg.EnableMetar {
			failures = append(failures, metarFeed.ApplySession(ctx, id, airport, time.Now().UTC()))
		}
		if cfg.EnableECFMP {
			failures = append(failures, (ecfmp.CandidateApply{Source: r.source, Session: cluster.EcfmpSessionAdapter{Writer: writer}}).ApplySession(ctx, id, time.Now().UTC()))
		}
		return errors.Join(failures...)
	}
	update, disconnect := r.work.SessionUpdate, r.work.SessionDisconnect
	r.work.SessionUpdate = func(ctx context.Context, id int32) error {
		return errors.Join(update(ctx, id), r.cdm.Recalculate(ctx, id))
	}
	r.work.SessionDisconnect = func(ctx context.Context, id int32) error {
		return errors.Join(disconnect(ctx, id), r.cdm.Recalculate(ctx, id))
	}
	return nil
}
func containsAirport(values []string, icao string) bool {
	for _, v := range values {
		if v == icao {
			return true
		}
	}
	return false
}

func (r *natsRuntime) liveSessionWorker(work func(context.Context, int32) error) func(context.Context, int32) error {
	return func(ctx context.Context, id int32) error {
		state, err := r.projection.Read(sessionNATSRef(id))
		if err != nil {
			return err
		}
		seed := state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(id)].GetValue().GetSession()
		if seed == nil || seed.Name != "LIVE" {
			return nil
		}
		return work(ctx, id)
	}
}
func natsReply(reply *pb.CommandReply) error {
	if reply == nil || reply.Status != pb.CommandReply_COMMITTED && reply.Status != pb.CommandReply_PENDING || reply.GetOutcome().GetStatus() == pb.CommandOutcome_FAILED {
		return fmt.Errorf("command unavailable: %s %s %s", reply.GetStatus(), reply.GetOutcome().GetReasonCode(), reply.GetDetail())
	}
	return nil
}
