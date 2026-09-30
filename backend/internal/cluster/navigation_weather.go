package cluster

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"FlightStrips/internal/vatsim"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// BinaryObjects is implemented by the FS_OBJECTS NATS Object Store. Keeping
// this boundary small also permits fault injection without an HTTP provider.
type BinaryObjects interface {
	GetBytes(string) ([]byte, error)
	PutBytes(string, []byte) (*nats.ObjectInfo, error)
}

// NATSObjects adapts FS_OBJECTS without exposing optional reads or writes to
// provider workers. Object identity and digest are verified above this layer.
type NATSObjects struct{ Store nats.ObjectStore }

func (o NATSObjects) GetBytes(name string) ([]byte, error) { return o.Store.GetBytes(name) }
func (o NATSObjects) PutBytes(name string, data []byte) (*nats.ObjectInfo, error) {
	return o.Store.PutBytes(name, data)
}

// NavigationWeather is opt-in. The current PostgreSQL application never
// constructs it; task 20 supplies its owner-routed writer and object store.
type NavigationWeather struct {
	Writer  Writer
	Objects BinaryObjects
}

func objectBytes(value *pb.ObjectValue) ([]byte, error) {
	if value == nil || value.SchemaVersion != 1 || value.GetContent() == nil {
		return nil, fmt.Errorf("invalid object envelope")
	}
	if err := validateTyped(value.ProtoReflect()); err != nil {
		return nil, err
	}
	data, err := (proto.MarshalOptions{Deterministic: true}).Marshal(value)
	if err != nil || len(data) > MaxObjectBytes {
		return nil, fmt.Errorf("invalid or oversized typed object")
	}
	return data, nil
}

func (a NavigationWeather) put(name string, value *pb.ObjectValue) (string, error) {
	if a.Objects == nil {
		return "", fmt.Errorf("object store unavailable")
	}
	data, err := objectBytes(value)
	if err != nil {
		return "", err
	}
	sha := digest(data)
	if !strings.HasSuffix(name, "/"+sha) {
		return "", fmt.Errorf("object name does not match digest")
	}
	prior, err := a.Objects.GetBytes(name)
	if err == nil {
		if !bytes.Equal(prior, data) {
			return "", ErrImmutableSnapshotCollision
		}
	} else if errors.Is(err, nats.ErrObjectNotFound) {
		if _, err = a.Objects.PutBytes(name, data); err != nil {
			return "", err
		}
	} else {
		return "", err
	}
	if _, err = a.readObject(name, sha); err != nil {
		return "", err
	}
	return sha, nil
}

func (a NavigationWeather) readObject(name, sha string) (*pb.ObjectValue, error) {
	if a.Objects == nil || len(sha) != 64 || !strings.HasSuffix(name, "/"+sha) {
		return nil, fmt.Errorf("invalid object reference")
	}
	data, err := a.Objects.GetBytes(name)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxObjectBytes || digest(data) != sha {
		return nil, fmt.Errorf("object checksum mismatch")
	}
	value := &pb.ObjectValue{}
	if err := pb.UnmarshalStrict(data, value); err != nil {
		return nil, err
	}
	if _, err := objectBytes(value); err != nil {
		return nil, err
	}
	return value, nil
}

func navKind(data *pb.NavData) (string, error) {
	if data == nil {
		return "", fmt.Errorf("nil navigation fragment")
	}
	switch data.GetFragment().(type) {
	case *pb.NavData_AirportFragment:
		return "airport", nil
	case *pb.NavData_ProcedureFragment:
		return "procedure", nil
	case *pb.NavData_FixFragment:
		return "fix", nil
	case *pb.NavData_TerminalFragment:
		return "terminal", nil
	case *pb.NavData_RouteCandidate:
		return "route", nil
	default:
		return "", fmt.Errorf("missing navigation fragment")
	}
}

func validateNav(data *pb.NavData) (string, error) {
	kind, err := navKind(data)
	if err != nil {
		return "", err
	}
	if _, err = Subject(airportRef(data.Airport)); err != nil {
		return "", err
	}
	if data.Version == nil || data.Version.Cycle == "" || data.SchemaVersion == "" || data.Provenance == nil || data.ImportedAt == nil || data.ValidationState == "" {
		return "", fmt.Errorf("incomplete navigation provenance")
	}
	if len(data.Digest) != 64 {
		return "", fmt.Errorf("invalid navigation domain digest")
	}
	if _, err := hex.DecodeString(data.Digest); err != nil {
		return "", fmt.Errorf("invalid navigation domain digest")
	}
	if err := validateTyped(data.ProtoReflect()); err != nil {
		return "", err
	}
	return kind, nil
}

// PublishNav writes a typed, content-addressed fragment and verifies read-back.
// NavData.digest retains the source domain's canonical digest; the returned
// SHA covers the complete stored ObjectValue, as required by the contract.
func (a NavigationWeather) PublishNav(data *pb.NavData) (*pb.NavObjectRef, error) {
	kind, err := validateNav(data)
	if err != nil {
		return nil, err
	}
	value := &pb.ObjectValue{SchemaVersion: 1, Content: &pb.ObjectValue_Nav{Nav: proto.Clone(data).(*pb.NavData)}}
	b, err := objectBytes(value)
	if err != nil {
		return nil, err
	}
	sha := digest(b)
	name := "nav/" + sha
	if _, err := a.put(name, value); err != nil {
		return nil, err
	}
	return &pb.NavObjectRef{Kind: kind, ObjectName: name, Sha256: sha}, nil
}

func (a NavigationWeather) ReadNav(ref *pb.NavObjectRef, airport string) (*pb.NavData, error) {
	if ref == nil || !strings.HasPrefix(ref.ObjectName, "nav/") {
		return nil, fmt.Errorf("invalid navigation reference")
	}
	value, err := a.readObject(ref.ObjectName, ref.Sha256)
	if err != nil {
		return nil, err
	}
	data := value.GetNav()
	kind, err := validateNav(data)
	if err != nil || kind != ref.Kind || data.Airport != airport {
		return nil, fmt.Errorf("navigation object identity mismatch")
	}
	return proto.Clone(data).(*pb.NavData), nil
}

func (a NavigationWeather) PublishProvider(page *pb.ProviderPage) (string, string, error) {
	if err := validateProviderPage(page); err != nil {
		return "", "", err
	}
	value := &pb.ObjectValue{SchemaVersion: 1, Content: &pb.ObjectValue_ProviderPage{ProviderPage: proto.Clone(page).(*pb.ProviderPage)}}
	b, err := objectBytes(value)
	if err != nil {
		return "", "", err
	}
	sha := digest(b)
	name := "provider/" + page.Provider + "/" + sha
	_, err = a.put(name, value)
	return name, sha, err
}

func (a NavigationWeather) ReadProvider(name, sha, provider, resource string) (*pb.ProviderPage, error) {
	if provider == "" || !strings.HasPrefix(name, "provider/"+provider+"/") {
		return nil, fmt.Errorf("invalid provider reference")
	}
	value, err := a.readObject(name, sha)
	if err != nil {
		return nil, err
	}
	page := value.GetProviderPage()
	if page == nil || page.Provider != provider || page.Resource != resource || page.GetParsed() == nil {
		return nil, fmt.Errorf("provider page identity mismatch")
	}
	if err := validateProviderPage(page); err != nil {
		return nil, err
	}
	return proto.Clone(page).(*pb.ProviderPage), nil
}

func validateProviderPage(page *pb.ProviderPage) error {
	if page == nil || page.Provider == "" || page.Resource == "" || page.GetParsed() == nil || strings.ContainsAny(page.Provider, "/.") {
		return fmt.Errorf("invalid typed provider page")
	}
	if err := validateTyped(page.ProtoReflect()); err != nil {
		return err
	}
	switch content := page.GetParsed().(type) {
	case *pb.ProviderPage_Transceivers:
		if page.Provider != "vatsim" || page.Resource != "transceivers/v3" {
			return fmt.Errorf("invalid transceiver provider identity")
		}
		if err := vatsim.ValidateTransceiverPage(content.Transceivers); err != nil {
			return err
		}
	case *pb.ProviderPage_Airac:
		if content.Airac == nil {
			return fmt.Errorf("empty AIRAC page")
		}
		for _, data := range content.Airac.Fragments {
			if _, err := validateNav(data); err != nil {
				return err
			}
		}
	case *pb.ProviderPage_Vatsim:
		if content.Vatsim == nil {
			return fmt.Errorf("empty VATSIM page")
		}
		for _, item := range content.Vatsim.Observations {
			if item == nil || item.ProviderId == "" || item.ObservedAt == nil {
				return fmt.Errorf("invalid VATSIM observation")
			}
		}
		if len(content.Vatsim.Flights) != 0 || content.Vatsim.SnapshotAt != nil {
			if content.Vatsim.SnapshotAt == nil || content.Vatsim.SnapshotAt.CheckValid() != nil {
				return fmt.Errorf("invalid VATSIM snapshot time")
			}
			seen := map[string]bool{}
			for _, flight := range content.Vatsim.Flights {
				if flight == nil || flight.Cid == "" || flight.Callsign == "" || seen[flight.Callsign] || flight.FlightPlan == nil || flight.State != "online" && flight.State != "prefile" || math.IsNaN(flight.Latitude) || math.IsNaN(flight.Longitude) || math.IsInf(flight.Latitude, 0) || math.IsInf(flight.Longitude, 0) || math.Abs(flight.Latitude) > 90 || math.Abs(flight.Longitude) > 180 {
					return fmt.Errorf("invalid VATSIM flight")
				}
				seen[flight.Callsign] = true
			}
		}
	case *pb.ProviderPage_Weather:
		if content.Weather == nil {
			return fmt.Errorf("empty weather page")
		}
		for _, item := range content.Weather.Observations {
			if item == nil {
				return fmt.Errorf("nil weather observation")
			}
		}
	case *pb.ProviderPage_Ecfmp:
		if content.Ecfmp == nil || content.Ecfmp.FetchedAt == nil || content.Ecfmp.FetchedAt.CheckValid() != nil {
			return fmt.Errorf("invalid ECFMP page timestamp")
		}
		for _, measure := range content.Ecfmp.Measures {
			if measure == nil || measure.Id <= 0 || measure.Kind == "" || measure.StartTime == nil || measure.EndTime == nil || measure.StartTime.CheckValid() != nil || measure.EndTime.CheckValid() != nil || !measure.EndTime.AsTime().After(measure.StartTime.AsTime()) {
				return fmt.Errorf("invalid ECFMP measure")
			}
			for _, filter := range measure.Filters {
				if filter == nil || filter.Kind == "" {
					return fmt.Errorf("invalid ECFMP filter")
				}
			}
		}
	case *pb.ProviderPage_AtisFeed:
		if content.AtisFeed == nil || content.AtisFeed.FetchedAt == nil || content.AtisFeed.FetchedAt.CheckValid() != nil {
			return fmt.Errorf("invalid ATIS feed timestamp")
		}
		seen := map[string]bool{}
		for _, airport := range content.AtisFeed.Airports {
			if airport == nil || len(airport.Airport) != 4 || seen[airport.Airport] {
				return fmt.Errorf("invalid ATIS feed airport")
			}
			if _, err := Subject(airportRef(airport.Airport)); err != nil {
				return err
			}
			seen[airport.Airport] = true
			for _, entry := range []*pb.AtisFeedEntry{airport.Arrival, airport.Departure} {
				if entry != nil && (entry.Callsign == "" || entry.LastUpdated == nil || entry.LastUpdated.CheckValid() != nil) {
					return fmt.Errorf("invalid ATIS feed entry")
				}
			}
		}
	case *pb.ProviderPage_OpenMeteo:
		weather := content.OpenMeteo
		if weather == nil || weather.SourceId == "" || weather.SourceRevision == "" || weather.ObservedAt == nil || weather.ExpiresAt == nil || weather.ObservedAt.CheckValid() != nil || weather.ExpiresAt.CheckValid() != nil || !weather.ExpiresAt.AsTime().After(weather.ObservedAt.AsTime()) || len(weather.Samples) == 0 {
			return fmt.Errorf("invalid Open-Meteo page")
		}
		for _, sample := range weather.Samples {
			if sample == nil || sample.ForecastAt == nil || sample.ForecastAt.CheckValid() != nil || math.IsNaN(sample.LatitudeDegrees) || math.IsNaN(sample.LongitudeDegrees) || math.Abs(sample.LatitudeDegrees) > 90 || math.Abs(sample.LongitudeDegrees) > 180 || len(sample.Levels) == 0 {
				return fmt.Errorf("invalid Open-Meteo sample")
			}
			for _, level := range sample.Levels {
				if level == nil || math.IsNaN(level.AltitudeFeet) || math.IsNaN(level.EastKnots) || math.IsNaN(level.NorthKnots) || math.IsInf(level.AltitudeFeet, 0) || math.IsInf(level.EastKnots, 0) || math.IsInf(level.NorthKnots, 0) {
					return fmt.Errorf("invalid Open-Meteo wind level")
				}
			}
		}
	case *pb.ProviderPage_CdmConfig:
		config := content.CdmConfig
		if config == nil || len(config.Airport) != 4 || config.FetchedAt == nil || config.FetchedAt.CheckValid() != nil || config.DefaultRate <= 0 || config.DefaultRateLvo <= 0 || config.DefaultTaxiMinutes <= 0 || config.Deice == nil {
			return fmt.Errorf("invalid CDM configuration page")
		}
		if page.Resource != "airport/"+config.Airport {
			return fmt.Errorf("CDM configuration airport mismatch")
		}
		for _, rate := range config.Rates {
			if rate == nil {
				return fmt.Errorf("nil CDM rate")
			}
		}
		for _, interval := range config.SidIntervals {
			if interval == nil || math.IsNaN(interval.Value) || math.IsInf(interval.Value, 0) || interval.Value < 0 {
				return fmt.Errorf("invalid CDM SID interval")
			}
		}
		for _, zone := range config.TaxiZones {
			if zone == nil || zone.Minutes < 0 {
				return fmt.Errorf("invalid CDM taxi zone")
			}
			for _, point := range zone.Polygon {
				if point == nil || math.IsNaN(point.Latitude) || math.IsNaN(point.Longitude) || math.IsInf(point.Latitude, 0) || math.IsInf(point.Longitude, 0) || math.Abs(point.Latitude) > 90 || math.Abs(point.Longitude) > 180 {
					return fmt.Errorf("invalid CDM taxi point")
				}
			}
		}
		for _, delay := range config.Delays {
			if delay == nil {
				return fmt.Errorf("nil CDM delay")
			}
		}
		for _, platform := range config.Deice.Platforms {
			if platform == nil {
				return fmt.Errorf("nil CDM deice platform")
			}
		}
	case *pb.ProviderPage_ViffFlights:
		flights := content.ViffFlights
		if page.Provider != "viff" || flights == nil || len(flights.Airport) != 4 || flights.FetchedAt == nil || flights.FetchedAt.CheckValid() != nil || !strings.HasPrefix(page.Resource, "session/") {
			return fmt.Errorf("invalid vIFF flight page")
		}
		seen := map[string]bool{}
		for _, row := range flights.Flights {
			if row == nil || row.Callsign == "" || seen[row.Callsign] || row.Departure != flights.Airport || row.CdmData == nil || row.TaxiMinutes < 0 {
				return fmt.Errorf("invalid vIFF flight row")
			}
			seen[row.Callsign] = true
		}
	case *pb.ProviderPage_ViffMasters:
		masters := content.ViffMasters
		if page.Provider != "viff" || page.Resource != "airport-masters" || masters == nil || masters.FetchedAt == nil || masters.FetchedAt.CheckValid() != nil {
			return fmt.Errorf("invalid vIFF master page")
		}
		seen := map[string]bool{}
		for _, master := range masters.Masters {
			if master == nil || len(master.Airport) != 4 || master.Position == "" || seen[master.Airport] {
				return fmt.Errorf("invalid vIFF master row")
			}
			seen[master.Airport] = true
		}
	case *pb.ProviderPage_Hoppie:
		poll := content.Hoppie
		if page.Provider != "hoppie" || poll == nil || poll.Station == "" || poll.Station != strings.ToUpper(poll.Station) || page.Resource != "station/"+poll.Station || !canonicalUUID(poll.PollId) || poll.ObservedAt == nil || poll.ObservedAt.CheckValid() != nil {
			return fmt.Errorf("invalid Hoppie poll page")
		}
		seen := map[string]bool{}
		for _, message := range poll.Messages {
			if err := ValidatePdcProviderMessage(message); err != nil {
				return err
			}
			if message.To != poll.Station || seen[message.MessageId] {
				return fmt.Errorf("invalid Hoppie poll identity")
			}
			seen[message.MessageId] = true
		}
	default:
		return fmt.Errorf("unsupported typed provider page")
	}
	return nil
}

func manifestDigest(manifest *pb.NavManifest) string {
	copy := proto.Clone(manifest).(*pb.NavManifest)
	copy.Digest = ""
	b, _ := (proto.MarshalOptions{Deterministic: true}).Marshal(copy)
	return digest(b)
}

func (a NavigationWeather) read(ctx context.Context, ref *pb.AggregateRef) (*Aggregate, error) {
	subject, err := Subject(ref)
	if err != nil {
		return nil, err
	}
	return a.Writer.load(ctx, subject, ref)
}

func (a NavigationWeather) upsert(ctx context.Context, ref *pb.AggregateRef, id string, key string, value *pb.EntityRecord, verify func() error) (*pb.CommandReply, error) {
	if !canonicalUUID(id) {
		return nil, fmt.Errorf("invalid workflow ID")
	}
	if verify != nil {
		if err := verify(); err != nil {
			return nil, err
		}
	}
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "navigation-weather"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: key, Value: value}}}}}
	w := a.Writer
	w.Plan = func(_ context.Context, _ *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		kind, _ := recordKind(value)
		old := state.Indexes[kind][key]
		revision := uint64(0)
		if old != nil {
			revision = old.Revision
		}
		if verify != nil {
			if err := verify(); err != nil {
				return nil, pb.CommandReply_UNAVAILABLE, revision, err
			}
		}
		if old != nil && proto.Equal(old.Value, value) {
			return &pb.DomainChange{}, pb.CommandReply_COMMITTED, revision, nil
		}
		return &pb.DomainChange{Changes: []*pb.EntityChange{{Key: key, Revision: revision + 1, Operation: &pb.EntityChange_Upsert{Upsert: proto.Clone(value).(*pb.EntityRecord)}}}}, pb.CommandReply_COMMITTED, revision, nil
	}
	reply := w.Execute(ctx, request)
	if reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		return reply, fmt.Errorf("typed state update: %s: %s", reply.Status, reply.Detail)
	}
	return reply, nil
}

// ActivateManifest verifies every object immediately before the CAS event.
// A failed or concurrent import leaves the previous active entity intact.
func (a NavigationWeather) ActivateManifest(ctx context.Context, id string, manifest *pb.NavManifest) (*pb.CommandReply, error) {
	if manifest == nil || !manifest.Active || manifest.Cycle == "" || len(manifest.Objects) == 0 || !canonicalUUID(id) || manifest.SourceRevision > 0 && len(manifest.SourceSha256) != 64 {
		return nil, fmt.Errorf("incomplete active manifest")
	}
	if _, err := Subject(airportRef(manifest.Airport)); err != nil {
		return nil, err
	}
	if manifest.Digest != manifestDigest(manifest) {
		return nil, fmt.Errorf("manifest digest mismatch")
	}
	if manifest.SourceRevision > 0 {
		if _, err := hex.DecodeString(manifest.SourceSha256); err != nil {
			return nil, fmt.Errorf("invalid manifest source digest")
		}
	}
	verify := func() error {
		seen := map[string]bool{}
		for _, ref := range manifest.Objects {
			if ref == nil || seen[ref.ObjectName] {
				return fmt.Errorf("duplicate or empty manifest object")
			}
			seen[ref.ObjectName] = true
			fragment, err := a.ReadNav(ref, manifest.Airport)
			if err != nil {
				return err
			}
			if fragment.Version.Cycle != manifest.Cycle {
				return fmt.Errorf("manifest cycle mismatch")
			}
		}
		return nil
	}
	if err := verify(); err != nil {
		return nil, err
	}
	ref := airportRef(manifest.Airport)
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "navigation-weather"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: manifest.Airport, Value: &pb.EntityRecord{Value: &pb.EntityRecord_NavManifest{NavManifest: manifest}}}}}}}
	w := a.Writer
	w.Plan = func(_ context.Context, _ *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		old := state.Indexes[pb.EntityKind_NAV_MANIFEST][manifest.Airport]
		current := uint64(0)
		if old != nil {
			current = old.Revision
			prior := old.GetValue().GetNavManifest()
			if prior.SourceRevision > manifest.SourceRevision || prior.SourceRevision == manifest.SourceRevision && prior.SourceRevision > 0 && prior.SourceSha256 != manifest.SourceSha256 {
				return nil, pb.CommandReply_REVISION_CONFLICT, current, fmt.Errorf("newer AIRAC source already active")
			}
			if proto.Equal(prior, manifest) {
				return &pb.DomainChange{}, pb.CommandReply_COMMITTED, current, nil
			}
		}
		if err := verify(); err != nil {
			return nil, pb.CommandReply_UNAVAILABLE, current, err
		}
		return &pb.DomainChange{Changes: []*pb.EntityChange{candidateUpsert(manifest.Airport, old, &pb.EntityRecord{Value: &pb.EntityRecord_NavManifest{NavManifest: manifest}})}}, pb.CommandReply_COMMITTED, current, nil
	}
	reply := w.Execute(ctx, request)
	if reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		return reply, fmt.Errorf("manifest activation: %s: %s", reply.Status, reply.Detail)
	}
	return reply, nil
}

func (a NavigationWeather) ActiveManifest(ctx context.Context, airport string) (*pb.NavManifest, error) {
	state, err := a.read(ctx, airportRef(airport))
	if err != nil {
		return nil, err
	}
	entry := state.Indexes[pb.EntityKind_NAV_MANIFEST][airport]
	if entry == nil {
		return nil, nil
	}
	manifest := entry.GetValue().GetNavManifest()
	if !manifest.Active || manifest.Digest != manifestDigest(manifest) {
		return nil, fmt.Errorf("corrupt active manifest")
	}
	for _, ref := range manifest.Objects {
		if _, err := a.ReadNav(ref, airport); err != nil {
			return nil, err
		}
	}
	return proto.Clone(manifest).(*pb.NavManifest), nil
}

func (a NavigationWeather) PutRouteCache(ctx context.Context, airport, id string, route *pb.NavRouteCache) (*pb.CommandReply, error) {
	if route == nil || route.RouteKey == "" || route.ResolverVersion == "" || route.SchemaVersion == "" {
		return nil, fmt.Errorf("incomplete route cache")
	}
	verify := func() error {
		data, err := a.ReadNav(&pb.NavObjectRef{Kind: "route", ObjectName: route.ObjectName, Sha256: route.Sha256}, airport)
		if err != nil {
			return err
		}
		if data.GetRouteCandidate().ResolverVersion != route.ResolverVersion || data.GetRouteCandidate().SchemaVersion != route.SchemaVersion {
			return fmt.Errorf("route cache provenance mismatch")
		}
		return nil
	}
	return a.upsert(ctx, airportRef(airport), id, route.RouteKey, &pb.EntityRecord{Value: &pb.EntityRecord_NavRouteCache{NavRouteCache: route}}, verify)
}

func (a NavigationWeather) RouteCache(ctx context.Context, airport, key string) (*pb.NavRouteCache, error) {
	state, err := a.read(ctx, airportRef(airport))
	if err != nil {
		return nil, err
	}
	entry := state.Indexes[pb.EntityKind_NAV_ROUTE_CACHE][key]
	if entry == nil {
		return nil, nil
	}
	route := entry.GetValue().GetNavRouteCache()
	if route == nil || route.RouteKey != key || route.ResolverVersion == "" || route.SchemaVersion == "" {
		return nil, fmt.Errorf("corrupt route cache metadata")
	}
	data, err := a.ReadNav(&pb.NavObjectRef{Kind: "route", ObjectName: route.ObjectName, Sha256: route.Sha256}, airport)
	if err != nil {
		return nil, err
	}
	if data.GetRouteCandidate().ResolverVersion != route.ResolverVersion || data.GetRouteCandidate().SchemaVersion != route.SchemaVersion {
		return nil, fmt.Errorf("route cache provenance mismatch")
	}
	return proto.Clone(route).(*pb.NavRouteCache), nil
}

func (a NavigationWeather) PutCheckpoint(ctx context.Context, airport, id string, checkpoint *pb.ProviderCheckpoint) (*pb.CommandReply, error) {
	return a.PutCheckpointFor(ctx, airportRef(airport), id, checkpoint)
}

// PutCheckpointFor permits global-owned feeds to keep one typed source
// checkpoint shared by every airport. AIRAC pages remain airport-owned.
func (a NavigationWeather) PutCheckpointFor(ctx context.Context, ref *pb.AggregateRef, id string, checkpoint *pb.ProviderCheckpoint) (*pb.CommandReply, error) {
	if checkpoint == nil || checkpoint.Provider == "" || checkpoint.Resource == "" {
		return nil, fmt.Errorf("incomplete provider checkpoint")
	}
	if _, err := Subject(ref); err != nil || ref.GetSession() != nil && checkpoint.Provider != "viff" && checkpoint.Provider != "hoppie" {
		return nil, fmt.Errorf("provider checkpoint requires global or airport owner, except session vIFF or Hoppie")
	}
	verify := func() error {
		if checkpoint.ObjectName == "" && checkpoint.Sha256 == "" {
			return nil
		}
		_, err := a.ReadProvider(checkpoint.ObjectName, checkpoint.Sha256, checkpoint.Provider, checkpoint.Resource)
		return err
	}
	return a.upsert(ctx, ref, id, checkpoint.Provider+"."+checkpoint.Resource, &pb.EntityRecord{Value: &pb.EntityRecord_ProviderCheckpoint{ProviderCheckpoint: checkpoint}}, verify)
}

func (a NavigationWeather) Checkpoint(ctx context.Context, airport, provider, resource string) (*pb.ProviderCheckpoint, *pb.ProviderPage, error) {
	return a.CheckpointFor(ctx, airportRef(airport), provider, resource)
}

func (a NavigationWeather) CheckpointFor(ctx context.Context, ref *pb.AggregateRef, provider, resource string) (*pb.ProviderCheckpoint, *pb.ProviderPage, error) {
	checkpoint, page, _, err := a.CheckpointRevisionFor(ctx, ref, provider, resource)
	return checkpoint, page, err
}

func (a NavigationWeather) CheckpointRevisionFor(ctx context.Context, ref *pb.AggregateRef, provider, resource string) (*pb.ProviderCheckpoint, *pb.ProviderPage, uint64, error) {
	if _, err := Subject(ref); err != nil || ref.GetSession() != nil && provider != "viff" && provider != "hoppie" {
		return nil, nil, 0, fmt.Errorf("provider checkpoint requires global or airport owner, except session vIFF or Hoppie")
	}
	state, err := a.read(ctx, ref)
	if err != nil {
		return nil, nil, 0, err
	}
	entry := state.Indexes[pb.EntityKind_PROVIDER_CHECKPOINT][provider+"."+resource]
	if entry == nil {
		return nil, nil, 0, nil
	}
	checkpoint := entry.GetValue().GetProviderCheckpoint()
	if checkpoint == nil || checkpoint.Provider != provider || checkpoint.Resource != resource {
		return nil, nil, 0, fmt.Errorf("corrupt provider checkpoint")
	}
	copy := proto.Clone(checkpoint).(*pb.ProviderCheckpoint)
	if checkpoint.ObjectName == "" && checkpoint.Sha256 == "" {
		return copy, nil, entry.Revision, nil
	}
	page, err := a.ReadProvider(checkpoint.ObjectName, checkpoint.Sha256, provider, resource)
	if err != nil {
		return nil, nil, 0, err
	}
	return copy, page, entry.Revision, nil
}

// FetchProviderPageFenced gives AIRAC and VATSIM importers a durable typed
// checkpoint. Fetch may return the prior page for a conditional HTTP 304. A
// completed provider call publishes and verifies its typed page object before
// the airport owner advances the checkpoint with the stable result command ID.
func (a NavigationWeather) FetchProviderPageFenced(ctx context.Context, worker ExternalCallWorker, workflowID, airport, provider, resource string, fetch func(context.Context, *pb.ProviderCheckpoint, *pb.ProviderPage) (*pb.ProviderPage, *pb.ProviderCheckpoint, error)) (bool, error) {
	return a.FetchProviderPageFor(ctx, worker, workflowID, airportRef(airport), provider, resource, fetch)
}

// FetchProviderPageFor uses the owning aggregate for both intent and
// checkpoint. It is global for VATSIM/ECFMP and airport for AIRAC.
func (a NavigationWeather) FetchProviderPageFor(ctx context.Context, worker ExternalCallWorker, workflowID string, ref *pb.AggregateRef, provider, resource string, fetch func(context.Context, *pb.ProviderCheckpoint, *pb.ProviderPage) (*pb.ProviderPage, *pb.ProviderCheckpoint, error)) (bool, error) {
	return a.FetchProviderPageReserved(ctx, worker, workflowID, ref, provider, resource, nil, fetch)
}

// FetchProviderPageReserved commits the source intent before reserving a
// global quota slot. An uncertain reservation or provider response is never
// permission for the next owner to call again with the same workflow ID.
func (a NavigationWeather) FetchProviderPageReserved(ctx context.Context, worker ExternalCallWorker, workflowID string, ref *pb.AggregateRef, provider, resource string, reserve func(context.Context) (bool, error), fetch func(context.Context, *pb.ProviderCheckpoint, *pb.ProviderPage) (*pb.ProviderPage, *pb.ProviderCheckpoint, error)) (bool, error) {
	if fetch == nil || provider == "" || resource == "" {
		return false, fmt.Errorf("invalid fenced provider fetch")
	}
	prior, priorPage, err := a.CheckpointFor(ctx, ref, provider, resource)
	if err != nil {
		return false, err
	}
	var next *pb.ProviderCheckpoint
	return worker.Run(ctx, ExternalCallSpec{
		Source: ref, Destination: ref, WorkflowID: workflowID,
		Step:    "external/provider/" + provider + "/" + resource,
		Reserve: reserve,
		Fetch: func(ctx context.Context) (proto.Message, error) {
			page, checkpoint, err := fetch(ctx, prior, priorPage)
			if err != nil {
				return nil, err
			}
			if page == nil || page.Provider != provider || page.Resource != resource || checkpoint == nil || checkpoint.Provider != provider || checkpoint.Resource != resource {
				return nil, fmt.Errorf("provider page or checkpoint identity mismatch")
			}
			next = proto.Clone(checkpoint).(*pb.ProviderCheckpoint)
			return page, nil
		},
		Commit: func(ctx context.Context, commandID string, value proto.Message) *pb.CommandReply {
			page, ok := value.(*pb.ProviderPage)
			if !ok || next == nil {
				return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT}
			}
			name, sha, err := a.PublishProvider(page)
			if err != nil {
				return &pb.CommandReply{Status: pb.CommandReply_UNAVAILABLE, Detail: err.Error()}
			}
			next.ObjectName, next.Sha256 = name, sha
			reply, err := a.PutCheckpointFor(ctx, ref, commandID, next)
			if err != nil {
				return &pb.CommandReply{Status: pb.CommandReply_UNAVAILABLE, Detail: err.Error()}
			}
			return reply
		},
	})
}

func (a NavigationWeather) PutWeather(ctx context.Context, id string, cache *pb.WeatherCache) (*pb.CommandReply, error) {
	if cache == nil || cache.Provider == "" || cache.Observation == nil || cache.FetchedAt == nil || cache.ExpiresAt == nil || !cache.ExpiresAt.AsTime().After(cache.FetchedAt.AsTime()) {
		return nil, fmt.Errorf("invalid weather cache")
	}
	if _, err := Subject(airportRef(cache.Airport)); err != nil {
		return nil, err
	}
	return a.upsert(ctx, airportRef(cache.Airport), id, cache.Airport+"."+cache.Provider, &pb.EntityRecord{Value: &pb.EntityRecord_WeatherCache{WeatherCache: cache}}, nil)
}

func (a NavigationWeather) Weather(ctx context.Context, airport, provider string, now time.Time) (*pb.WeatherCache, error) {
	state, err := a.read(ctx, airportRef(airport))
	if err != nil {
		return nil, err
	}
	entry := state.Indexes[pb.EntityKind_WEATHER_CACHE][airport+"."+provider]
	if entry == nil {
		return nil, nil
	}
	cache := entry.GetValue().GetWeatherCache()
	if cache == nil || cache.Observation == nil || cache.FetchedAt == nil || cache.ExpiresAt == nil || !cache.ExpiresAt.AsTime().After(cache.FetchedAt.AsTime()) {
		return nil, fmt.Errorf("corrupt weather cache")
	}
	if !now.Before(cache.ExpiresAt.AsTime()) {
		return nil, nil
	}
	return proto.Clone(cache).(*pb.WeatherCache), nil
}

// ReserveQuota consumes one global slot before a provider call. The stable
// workflow UUID is the command ID in the durable global ledger. A retry after
// an uncertain response returns fresh=false, so it cannot issue another call.
func (a NavigationWeather) ReserveQuota(ctx context.Context, workflowID, provider string, window time.Time, limit uint32) (fresh bool, err error) {
	if !canonicalUUID(workflowID) || provider == "" || strings.Contains(provider, ".") || limit == 0 || window.IsZero() {
		return false, fmt.Errorf("invalid quota reservation")
	}
	window = window.UTC().Truncate(time.Second)
	key := fmt.Sprintf("%s.%d", provider, window.Unix())
	quota := &pb.ProviderQuota{Provider: provider, WindowStart: timestamppb.New(window), Limit: limit}
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: workflowID, Aggregate: globalRef(), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "provider-quota"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: key, Value: &pb.EntityRecord{Value: &pb.EntityRecord_ProviderQuota{ProviderQuota: quota}}}}}}}
	// Check the ledger before attempting the command. An ambiguous publication
	// may already have consumed quota; never interpret it as permission to call.
	state, err := a.read(ctx, globalRef())
	if err != nil {
		return false, err
	}
	if previous := state.Ledger[workflowID]; previous != nil {
		hash, err := RequestHash(request)
		if err != nil || previous.RequestSha256 != hash {
			return false, fmt.Errorf("workflow ID has different quota request")
		}
		if previous.Status != pb.CommandOutcome_SUCCEEDED {
			return false, fmt.Errorf("quota reservation previously failed: %s", previous.ReasonCode)
		}
		return false, nil
	}
	w := a.Writer
	w.Plan = func(_ context.Context, _ *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		old := state.Indexes[pb.EntityKind_PROVIDER_QUOTA][key]
		current := uint64(0)
		used := uint32(0)
		if old != nil {
			current = old.Revision
			prior := old.GetValue().GetProviderQuota()
			if prior == nil || prior.Limit != limit {
				return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("quota limit changed")
			}
			used = prior.Used
		}
		if used >= limit || used == math.MaxUint32 {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("provider quota exhausted")
		}
		next := proto.Clone(quota).(*pb.ProviderQuota)
		next.Used = used + 1
		return &pb.DomainChange{Changes: []*pb.EntityChange{{Key: key, Revision: current + 1, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_ProviderQuota{ProviderQuota: next}}}}}}, pb.CommandReply_COMMITTED, current, nil
	}
	reply, fresh := w.ExecuteFresh(ctx, request)
	if reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		return false, fmt.Errorf("quota reservation: %s: %s", reply.Status, reply.Detail)
	}
	return fresh, nil
}

func (a NavigationWeather) Quota(ctx context.Context, provider string, window time.Time) (*pb.ProviderQuota, error) {
	state, err := a.read(ctx, globalRef())
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%s.%d", provider, window.UTC().Truncate(time.Second).Unix())
	entry := state.Indexes[pb.EntityKind_PROVIDER_QUOTA][key]
	if entry == nil {
		return nil, nil
	}
	return proto.Clone(entry.GetValue().GetProviderQuota()).(*pb.ProviderQuota), nil
}

// FetchWeather resumes safely after owner takeover: a consumed reservation
// never sends a second external request. The provider callback must decode its
// external format and return only a typed observation.
func (a NavigationWeather) FetchWeather(ctx context.Context, workflowID, airport, provider string, window time.Time, limit uint32, ttl time.Duration, fetch func(context.Context) (*pb.WeatherObservation, error)) (bool, error) {
	return a.FetchWeatherFenced(ctx, ExternalCallWorker{Writer: a.Writer}, workflowID, airport, provider, ttl,
		func(ctx context.Context) (bool, error) {
			return a.ReserveQuota(ctx, workflowID, provider, window, limit)
		}, fetch)
}

// FetchWeatherFenced is the candidate worker path. The caller supplies an
// owner-routed global reservation; task 20 wires that route when the NATS-only
// runtime replaces the legacy poller. The airport intent is committed first,
// and a takeover never repeats an uncertain quota-consuming provider call.
func (a NavigationWeather) FetchWeatherFenced(ctx context.Context, worker ExternalCallWorker, workflowID, airport, provider string, ttl time.Duration, reserve func(context.Context) (bool, error), fetch func(context.Context) (*pb.WeatherObservation, error)) (bool, error) {
	if reserve == nil || fetch == nil || ttl <= 0 || provider == "" {
		return false, fmt.Errorf("invalid fenced weather fetch")
	}
	ref := airportRef(airport)
	if _, err := Subject(ref); err != nil {
		return false, err
	}
	return worker.Run(ctx, ExternalCallSpec{
		Source: ref, Destination: ref, WorkflowID: workflowID,
		Step:    "external/weather/" + provider,
		Reserve: reserve,
		Fetch:   func(ctx context.Context) (proto.Message, error) { return fetch(ctx) },
		Commit: func(ctx context.Context, commandID string, value proto.Message) *pb.CommandReply {
			observation, ok := value.(*pb.WeatherObservation)
			if !ok {
				return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT}
			}
			now := time.Now().UTC()
			reply, err := a.PutWeather(ctx, commandID, &pb.WeatherCache{Airport: airport, Provider: provider, Observation: observation, FetchedAt: timestamppb.New(now), ExpiresAt: timestamppb.New(now.Add(ttl))})
			if err != nil {
				return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT, Detail: err.Error()}
			}
			return reply
		},
	})
}
