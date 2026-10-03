package euroscopebinary

import (
	"FlightStrips/internal/cluster"
	"FlightStrips/internal/config"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"google.golang.org/protobuf/proto"
	"sync"
)

type syncEntityReader interface {
	ReadEntity(*pb.AggregateRef, pb.EntityKind, string) (*pb.EntitySnapshot, error)
}

// Keep only the latest wire view per live aircraft on this socket. Position
// updates don't alter that view and must not replay controller assignments.
type socketStripSync struct {
	mu     sync.Mutex
	latest map[string]*es.BackendSyncStrip
}

func (s *socketStripSync) initial(state *cluster.Aggregate) *es.Envelope {
	s.mu.Lock()
	defer s.mu.Unlock()
	frame := backendSync(state)
	s.latest = map[string]*es.BackendSyncStrip{}
	for _, strip := range frame.GetBackendSync().Strips {
		s.latest[strip.Callsign] = strip
	}
	return frame
}

func (s *socketStripSync) delta(reader syncEntityReader, delta *pb.FrontendDelta) (*es.Envelope, error) {
	if delta.GetAggregate().GetSession() == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.latest == nil {
		s.latest = map[string]*es.BackendSyncStrip{}
	}
	callsigns := map[string]bool{}
	for _, change := range delta.Changes {
		value := change.GetUpsert()
		if value.GetStrip() != nil || value.GetCdmState() != nil || value.GetEcfmpState() != nil {
			callsigns[change.Key] = true
		}
		if deleted := change.GetDelete(); deleted != nil {
			switch deleted.Kind {
			case pb.EntityKind_STRIP:
				delete(s.latest, change.Key)
			case pb.EntityKind_CDM_STATE, pb.EntityKind_ECFMP_STATE:
				callsigns[change.Key] = true
			}
		}
	}
	lat, lon := config.GetAirportCoordinates()
	out := &es.BackendSyncEvent{Latitude: lat, Longitude: lon}
	for callsign := range callsigns {
		entity, err := reader.ReadEntity(delta.Aggregate, pb.EntityKind_STRIP, callsign)
		if err != nil {
			return nil, err
		}
		strip := entity.GetValue().GetStrip()
		if !operationalStrip(strip) {
			delete(s.latest, callsign)
			continue
		}
		cdm, err := reader.ReadEntity(delta.Aggregate, pb.EntityKind_CDM_STATE, callsign)
		if err != nil {
			return nil, err
		}
		flow, err := reader.ReadEntity(delta.Aggregate, pb.EntityKind_ECFMP_STATE, callsign)
		if err != nil {
			return nil, err
		}
		view := syncStrip(strip)
		view.Cdm = syncCdm(strip, cdm.GetValue().GetCdmState(), flow.GetValue().GetEcfmpState())
		if proto.Equal(s.latest[callsign], view) {
			continue
		}
		s.latest[callsign] = view
		out.Strips = append(out.Strips, view)
	}
	if len(out.Strips) == 0 {
		return nil, nil
	}
	return &es.Envelope{Event: &es.Envelope_BackendSync{BackendSync: out}}, nil
}
