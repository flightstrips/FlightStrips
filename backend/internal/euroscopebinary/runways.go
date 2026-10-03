package euroscopebinary

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"google.golang.org/protobuf/proto"
)

// Reports are advisory, socket-local state. A new report replaces the old one;
// disconnect discards it. Canonical runways remain in the shared session record.
type socketRunwayReports struct {
	mu      sync.Mutex
	current *es.RunwayEvent
	last    *es.RunwayMismatchAlertEvent
}

func (r *socketRunwayReports) replace(report *es.RunwayEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current = proto.Clone(report).(*es.RunwayEvent)
}

func runwayReportIsMaster(state *cluster.Aggregate, connection string, epoch uint64) bool {
	return state.Master != nil && state.Owner != nil && state.Master.OwnerEpoch == state.Owner.Epoch &&
		state.Master.ConnectionId == connection && epoch != 0 && state.Master.Epoch == epoch
}

func (r *socketRunwayReports) evaluate(state *cluster.Aggregate, connection string) *es.Envelope {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.current == nil || state.Master == nil || state.Sync == nil ||
		state.Sync.ConnectionId != state.Master.ConnectionId || state.Sync.MasterEpoch != state.Master.Epoch {
		return nil
	}
	if runwayReportIsMaster(state, connection, state.Master.Epoch) {
		r.last = nil
		return nil
	}
	session := state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(state.Ref.GetSession().Id)].GetValue().GetSession()
	if session == nil {
		return nil
	}
	alert := &es.RunwayMismatchAlertEvent{}
	for _, runway := range session.Runways {
		if runway.Departure {
			alert.ExpectedDeparture = append(alert.ExpectedDeparture, runway.Name)
		}
		if runway.Arrival {
			alert.ExpectedArrival = append(alert.ExpectedArrival, runway.Name)
		}
	}
	for _, runway := range r.current.Runways {
		if runway.GetDeparture() {
			alert.CurrentDeparture = append(alert.CurrentDeparture, runway.Name)
		}
		if runway.GetArrival() {
			alert.CurrentArrival = append(alert.CurrentArrival, runway.Name)
		}
	}
	// Runway activation is a set: ordering, whitespace and case are immaterial.
	normalize := func(values []string) []string {
		for i := range values {
			values[i] = strings.ToUpper(strings.TrimSpace(values[i]))
		}
		slices.Sort(values)
		return slices.Compact(values)
	}
	alert.ExpectedDeparture = normalize(alert.ExpectedDeparture)
	alert.ExpectedArrival = normalize(alert.ExpectedArrival)
	alert.CurrentDeparture = normalize(alert.CurrentDeparture)
	alert.CurrentArrival = normalize(alert.CurrentArrival)
	if slices.Equal(alert.ExpectedDeparture, alert.CurrentDeparture) && slices.Equal(alert.ExpectedArrival, alert.CurrentArrival) {
		r.last = nil
		return nil
	}
	if proto.Equal(r.last, alert) {
		return nil
	}
	r.last = alert
	return &es.Envelope{SessionId: session.Id, OwnerEpoch: state.Owner.GetEpoch(), MasterEpoch: state.Master.Epoch,
		Event: &es.Envelope_RunwayMismatchAlert{RunwayMismatchAlert: alert}}
}
