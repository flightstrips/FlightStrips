package navigationcandidate

import (
	"context"
	"fmt"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/materializer"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/navdata/airacnet"
	"FlightStrips/internal/aman/terminal"
	pb "FlightStrips/pkg/events/cluster"
)

// Source reuses the operational AIRAC.NET parser and canonical materializer.
// The collector is private to one fenced provider call; no raw HTTP body or
// partial candidate is persisted before AiracCandidateWorker commits it.
type Source struct {
	Cycles     navdata.CycleSource
	Airports   navdata.AirportSource
	Runways    navdata.RunwaySource
	Procedures navdata.ProcedureSource
	Fixes      navdata.FixSource
	Routes     navdata.RouteResolver
	Terminal   terminal.Configuration
	Now        func() time.Time
}

func FromAiracNet(provider *airacnet.Adapter, config terminal.Configuration) Source {
	return Source{Cycles: provider, Airports: provider, Runways: config, Procedures: provider, Fixes: provider, Routes: provider, Terminal: config}
}

func (s Source) Fetch(ctx context.Context, airport string, _ *pb.ProviderCheckpoint, _ *pb.ProviderPage) (*pb.AiracPage, *pb.ProviderCheckpoint, error) {
	collector := &candidateCollector{}
	importer, err := materializer.New(materializer.Dependencies{Cycles: s.Cycles, Airports: s.Airports, Runways: s.Runways,
		Procedures: s.Procedures, Fixes: s.Fixes, Routes: s.Routes, Cache: collector, Terminal: s.Terminal, Now: s.Now})
	if err != nil {
		return nil, nil, err
	}
	if err := importer.Refresh(ctx, materializer.Request{Airport: navdata.AirportID(airport)}); err != nil {
		return nil, nil, err
	}
	if collector.manifest == nil || collector.airport == nil || collector.fixes == nil || collector.terminal == nil {
		return nil, nil, fmt.Errorf("AIRAC materializer returned an incomplete candidate")
	}
	page, err := candidatePage(airport, collector)
	if err != nil {
		return nil, nil, err
	}
	return page, &pb.ProviderCheckpoint{Etag: collector.manifest.Version.SourceRevision}, nil
}

type candidateCollector struct {
	airport    *navdata.CandidateAirportFragment
	fixes      *navdata.CandidateFixFragment
	procedures []navdata.CandidateProcedureFragment
	terminal   *navdata.CandidateTerminalFragment
	manifest   *navdata.ManifestCandidate
}

func (*candidateCollector) missing() error {
	return &aman.DomainError{Class: aman.ErrorNotFound, Message: "candidate has no active manifest"}
}
func (c *candidateCollector) ActiveManifest(context.Context, navdata.AirportID) (navdata.ActiveManifest, error) {
	return navdata.ActiveManifest{}, c.missing()
}
func (c *candidateCollector) ActiveVersion(context.Context, navdata.AirportID) (navdata.DatasetVersion, error) {
	return navdata.DatasetVersion{}, c.missing()
}
func (c *candidateCollector) Route(context.Context, navdata.RouteKey) (navdata.RouteGeometry, error) {
	return navdata.RouteGeometry{}, c.missing()
}
func (c *candidateCollector) TerminalPath(context.Context, navdata.AirportID, navdata.FeederID, aman.RunwayGroupID) (navdata.TerminalPath, error) {
	return navdata.TerminalPath{}, c.missing()
}
func (c *candidateCollector) PutAirportFragment(_ context.Context, value navdata.CandidateAirportFragment) (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	c.airport = &value
	return value.Digest, nil
}
func (c *candidateCollector) PutProcedureFragment(_ context.Context, value navdata.CandidateProcedureFragment) (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	c.procedures = append(c.procedures, value)
	return value.Digest, nil
}
func (c *candidateCollector) PutFixFragment(_ context.Context, value navdata.CandidateFixFragment) (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	c.fixes = &value
	return value.Digest, nil
}
func (c *candidateCollector) PutTerminalFragment(_ context.Context, value navdata.CandidateTerminalFragment) (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	c.terminal = &value
	return value.Digest, nil
}
func (c *candidateCollector) PutRoute(context.Context, navdata.RouteCandidate) (navdata.RouteKey, error) {
	return "", fmt.Errorf("AIRAC import cannot materialize a route")
}
func (c *candidateCollector) ActivateManifest(_ context.Context, value navdata.ManifestCandidate) (int64, error) {
	if err := value.Validate(); err != nil {
		return 0, err
	}
	c.manifest = &value
	return 1, nil
}
func (*candidateCollector) PruneNavigationCache(context.Context, navdata.AirportID) error { return nil }
