package amancandidate

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"fmt"
)

// navigation reads verified immutable fragments behind the accepted manifest.
// Route acquisition uses the existing resolver under a durable external intent;
// the evaluator never treats a process-local route cache as accepted authority.
type navigation struct {
	state    cluster.NavigationWeather
	worker   cluster.ExternalCallWorker
	resolver navdata.RouteResolver
	airport  string
	snapshot navdata.ActiveGeometrySnapshot
	config   terminal.Configuration
	tma      *terminal.TMAVolume
	routes   map[navdata.RouteKey]navdata.RouteGeometry
}

func loadNavigation(ctx context.Context, state cluster.NavigationWeather, worker cluster.ExternalCallWorker, resolver navdata.RouteResolver, airport string) (*navigation, error) {
	n := &navigation{state: state, worker: worker, resolver: resolver, airport: airport, routes: map[navdata.RouteKey]navdata.RouteGeometry{}}
	m, err := state.ActiveManifest(ctx, airport)
	if err != nil {
		return n, err
	}
	if m == nil {
		return n, fmt.Errorf("committed navigation manifest unavailable")
	}
	n.snapshot.Manifest = navdata.ManifestCandidate{Airport: navdata.AirportID(airport)}
	// The accepted manifest's source revision is stable across process restart.
	n.snapshot.ManifestRevision = int64(m.SourceRevision)
	for _, ref := range m.Objects {
		data, err := state.ReadNav(ref, airport)
		if err != nil {
			return n, err
		}
		if data.Version == nil {
			return n, fmt.Errorf("navigation version unavailable")
		}
		version := *decodeNavDatasetVersion(data.Version)
		if n.snapshot.Manifest.Version.Cycle != "" && !n.snapshot.Manifest.Version.Equal(version) {
			return n, fmt.Errorf("manifest mixes dataset versions")
		}
		n.snapshot.Manifest.Version = version
		switch {
		case data.GetAirportFragment() != nil:
			f := data.GetAirportFragment()
			if f.Airport == nil {
				return n, fmt.Errorf("navigation airport unavailable")
			}
			n.snapshot.Airport = *decodeNavAirport(f.Airport)
			n.snapshot.Manifest.AirportDigest = data.Digest
			for _, r := range f.Runways {
				n.snapshot.Runways = append(n.snapshot.Runways, *decodeNavRunway(r))
			}
		case data.GetFixFragment() != nil:
			n.snapshot.Manifest.FixDigest = data.Digest
			for _, f := range data.GetFixFragment().Fixes {
				n.snapshot.Fixes = append(n.snapshot.Fixes, *decodeNavFix(f))
			}
		case data.GetProcedureFragment() != nil:
			n.snapshot.Manifest.ProcedureDigests = append(n.snapshot.Manifest.ProcedureDigests, data.Digest)
			for _, p := range data.GetProcedureFragment().Procedures {
				n.snapshot.Procedures = append(n.snapshot.Procedures, *decodeNavProcedure(p))
				for _, h := range p.Holdings {
					n.snapshot.Holdings = append(n.snapshot.Holdings, *decodeNavHolding(h))
				}
			}
		case data.GetTerminalFragment() != nil:
			f := data.GetTerminalFragment()
			if f.OperationalPolicy == nil || f.OperationalPolicy.Airport != airport || f.OperationalPolicy.ConfigVersion != f.ConfigVersion {
				return n, fmt.Errorf("committed terminal policy unavailable or mismatched")
			}
			n.config = decodeTerminalPolicy(f.OperationalPolicy)
			if f.OperationalPolicy.TmaVolume != nil {
				volume, err := decodeTMAVolume(f.OperationalPolicy.TmaVolume)
				if err != nil {
					return n, err
				}
				n.tma = &volume
			} else if airport == "EKCH" {
				return n, fmt.Errorf("committed EKCH TMA boundary unavailable")
			}
			if err := n.config.ValidateOperationalSettings(); err != nil {
				return n, err
			}
			n.snapshot.TerminalVersion = f.ConfigVersion
			n.snapshot.Manifest.TerminalDigest = data.Digest
			for _, p := range f.Paths {
				n.snapshot.TerminalPaths = append(n.snapshot.TerminalPaths, *decodeNavTerminalPath(p))
			}
			for _, h := range f.Holdings {
				n.snapshot.Holdings = append(n.snapshot.Holdings, *decodeNavHolding(h))
			}
			for _, t := range f.TimelineMappings {
				n.snapshot.TimelineMappings = append(n.snapshot.TimelineMappings, *decodeAmanTimelineMapping(t))
			}
		}
	}
	if n.config.Airport == "" {
		return n, fmt.Errorf("committed terminal policy unavailable")
	}
	return n, n.snapshot.Manifest.Version.Validate()
}
func (n *navigation) ActiveGeometrySnapshot(_ context.Context, airport navdata.AirportID) (navdata.ActiveGeometrySnapshot, error) {
	if n.snapshot.Manifest.Version.Cycle == "" || string(airport) != n.airport {
		return navdata.ActiveGeometrySnapshot{}, fmt.Errorf("committed navigation unavailable")
	}
	return n.snapshot, nil
}
func (n *navigation) ActiveVersion(ctx context.Context, airport navdata.AirportID) (navdata.DatasetVersion, error) {
	s, e := n.ActiveGeometrySnapshot(ctx, airport)
	return s.Manifest.Version, e
}
func (n *navigation) TerminalPath(_ context.Context, airport navdata.AirportID, feeder navdata.FeederID, group aman.RunwayGroupID) (navdata.TerminalPath, error) {
	for _, p := range n.snapshot.TerminalPaths {
		if p.Airport == airport && p.RunwayGroup == group && (p.Feeder == feeder || string(p.FeederFix) == string(feeder)) {
			return p, nil
		}
	}
	return navdata.TerminalPath{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "committed terminal path unavailable"}
}
func (n *navigation) Route(ctx context.Context, key navdata.RouteKey) (navdata.RouteGeometry, error) {
	if r, ok := n.routes[key]; ok {
		return r, nil
	}
	c, err := n.state.RouteCache(ctx, n.airport, string(key))
	if err != nil {
		return navdata.RouteGeometry{}, err
	}
	if c == nil {
		return navdata.RouteGeometry{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "committed route unavailable"}
	}
	data, err := n.state.ReadNav(&pb.NavObjectRef{Kind: "route", ObjectName: c.ObjectName, Sha256: c.Sha256}, n.airport)
	if err != nil {
		return navdata.RouteGeometry{}, err
	}
	r := data.GetRouteCandidate()
	if r == nil || r.Geometry == nil {
		return navdata.RouteGeometry{}, fmt.Errorf("committed route geometry unavailable")
	}
	result := *decodeNavRouteGeometry(r.Geometry)
	if err = result.Validate(); err != nil {
		return navdata.RouteGeometry{}, err
	}
	n.routes[key] = result
	return result, nil
}
func (n *navigation) MaterializeRoute(ctx context.Context, q navdata.RouteQuery, resolverVersion string) (navdata.RouteKey, error) {
	candidate := navdata.RouteCandidate{Query: q, ResolverVersion: resolverVersion, SchemaVersion: navdata.CanonicalSchemaVersion}
	key, err := candidate.PersistenceKey()
	if err != nil {
		return "", err
	}
	if _, err := n.Route(ctx, key); err == nil {
		return key, nil
	}
	if n.resolver == nil {
		return "", fmt.Errorf("AIRAC route resolver unavailable")
	}
	resource := "route/" + string(key)
	id, err := cluster.ProviderEventCommandID("aman-route", "airacnet", n.airport+"/"+resource)
	if err != nil {
		return "", err
	}
	_, err = n.state.FetchProviderPageFor(ctx, n.worker, id, airportRef(n.airport), "airacnet", resource, func(ctx context.Context, _ *pb.ProviderCheckpoint, _ *pb.ProviderPage) (*pb.ProviderPage, *pb.ProviderCheckpoint, error) {
		geometry, err := n.resolver.Resolve(ctx, q)
		if err != nil {
			return nil, nil, err
		}
		if err = geometry.Validate(); err != nil {
			return nil, nil, err
		}
		d := &pb.NavData{Airport: n.airport, Version: encodeNavDatasetVersion(&q.Version), SchemaVersion: navdata.CanonicalSchemaVersion, Provenance: encodeNavProvenance(&geometry.Provenance), ImportedAt: timestamp(geometry.Provenance.ImportedAt), ValidatedAt: timestamp(geometry.Provenance.ImportedAt), ValidationState: "validated", Digest: geometry.Digest,
			Fragment: &pb.NavData_RouteCandidate{RouteCandidate: &pb.NavRouteCandidate{Query: encodeNavRouteQuery(&q), Geometry: encodeNavRouteGeometry(&geometry), ResolverVersion: resolverVersion, SchemaVersion: navdata.CanonicalSchemaVersion, CreatedAt: timestamp(geometry.Provenance.ImportedAt)}}}
		return &pb.ProviderPage{Provider: "airacnet", Resource: resource, Parsed: &pb.ProviderPage_Airac{Airac: &pb.AiracPage{Fragments: []*pb.NavData{d}}}}, &pb.ProviderCheckpoint{Provider: "airacnet", Resource: resource}, nil
	})
	if err != nil {
		return "", err
	}
	_, page, err := n.state.Checkpoint(ctx, n.airport, "airacnet", resource)
	if err != nil {
		return "", err
	}
	if page == nil || page.GetAirac() == nil || len(page.GetAirac().Fragments) != 1 {
		return "", fmt.Errorf("committed route result unavailable")
	}
	data := page.GetAirac().Fragments[0]
	if data.GetRouteCandidate() == nil {
		return "", fmt.Errorf("invalid committed route result")
	}
	ref, err := n.state.PublishNav(data)
	if err != nil {
		return "", err
	}
	cacheID, err := cluster.ProviderEventCommandID("aman-route-cache", "airacnet", id)
	if err != nil {
		return "", err
	}
	if _, err = n.state.PutRouteCache(ctx, n.airport, cacheID, &pb.NavRouteCache{RouteKey: string(key), ResolverVersion: resolverVersion, SchemaVersion: navdata.CanonicalSchemaVersion, ObjectName: ref.ObjectName, Sha256: ref.Sha256}); err != nil {
		return "", err
	}
	_, err = n.Route(ctx, key)
	return key, err
}
