package cluster

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	"FlightStrips/internal/vatsim"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type transceiverReplica struct {
	nc         *nats.Conn
	projection *Projection
	owner      *OwnerRuntime
	nav        NavigationWeather
	stop       context.CancelFunc
}

func transceiverNATSConfig(t *testing.T, ctx context.Context) natsresources.Config {
	t.Helper()
	port := 4222
	if value := os.Getenv("NATS_TEST_PORT_BASE"); value != "" {
		parsed, err := strconv.Atoi(value)
		require.NoError(t, err)
		require.True(t, parsed >= 1024 && parsed <= 65533)
		port = parsed
	}
	urls := func(user string) []string {
		var urls []string
		for i := 0; i < 3; i++ {
			urls = append(urls, fmt.Sprintf("nats://%s:%s-local-only@127.0.0.1:%d", user, user, port+i))
		}
		return urls
	}
	cfg := natsresources.Config{URLs: urls("bootstrap"), ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
	admin, err := natsresources.Connect(cfg)
	require.NoError(t, err)
	defer admin.Close()
	require.NoError(t, natscluster.WaitForQuorum(ctx, admin))
	require.NoError(t, natsresources.Bootstrap(ctx, admin, cfg))
	cfg.URLs = urls("backend")
	return cfg
}

func newTransceiverReplica(t *testing.T, ctx context.Context, cfg natsresources.Config) *transceiverReplica {
	t.Helper()
	nc, err := natsresources.Connect(cfg)
	require.NoError(t, err)
	projection := startProjection(t, ctx, nc, cfg)
	store := NATSStore{JS: projection.JS}
	owner, err := NewOwnerRuntime(nc, projection, store)
	require.NoError(t, err)
	require.NoError(t, owner.Track(globalRef()))
	objects, err := projection.JS.ObjectStore("FS_OBJECTS")
	require.NoError(t, err)
	runCtx, stop := context.WithCancel(ctx)
	node := &transceiverReplica{nc: nc, projection: projection, owner: owner, nav: NavigationWeather{Writer: Writer{Store: store, NodeID: owner.NodeID, Projection: projection, Lease: owner}, Objects: NATSObjects{Store: objects}}, stop: stop}
	go func() { _ = owner.Run(runCtx) }()
	t.Cleanup(func() { stop(); nc.Close() })
	return node
}

func waitTransceiverOwner(t *testing.T, ctx context.Context, nodes []*transceiverReplica, ref *pb.AggregateRef) *transceiverReplica {
	t.Helper()
	for ctx.Err() == nil {
		for _, node := range nodes {
			if node.owner.CanWrite(ref) {
				return node
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("owner takeover timed out")
	return nil
}

// Kill after the actual JetStream checkpoint commit, before the writer can
// report it or complete its source intent. The survivor must prove the result
// using the stable command ledger UUID rather than sending another HTTP call.
type transceiverResultDeathStore struct {
	EventStore
	die   func()
	armed bool
}

func (s *transceiverResultDeathStore) Publish(ctx context.Context, subject string, expected uint64, data []byte) (uint64, error) {
	sequence, err := s.EventStore.Publish(ctx, subject, expected, data)
	if err == nil && s.armed {
		var event pb.StateEvent
		if proto.Unmarshal(data, &event) == nil {
			for _, change := range event.GetDomainChanged().GetChanges() {
				if checkpoint := change.GetUpsert().GetProviderCheckpoint(); checkpoint != nil && checkpoint.Resource == transceiverResource {
					s.armed = false
					s.die()
					return 0, fmt.Errorf("owner died after result commit before PubAck")
				}
			}
		}
	}
	return sequence, err
}

// This planner exercises the typed sector write and source handoff contract.
// The operational frequency coverage/layout policy is owned by Task18c; Task20
// binds that planner in place of this small deterministic fixture policy.
func transceiverFixtureSectors(ctx context.Context, req *pb.CommandRequest, state *Aggregate, generation TransceiverGeneration) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	frequencies := generation.GetFrequencies("EKCH_TWR")
	if len(frequencies) == 0 {
		return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("fixture frequency unavailable")
	}
	request := proto.Clone(req).(*pb.CommandRequest)
	request.Command = &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_ReplaceSectorOwners{ReplaceSectorOwners: &pb.ReplaceSectorOwners{Owners: []*pb.SectorOwner{{Sector: "TWR", Position: frequencies[0], Identifier: "EKCH_TWR"}}}}}}
	return PlanControllerSector(ctx, request, state)
}

func TestTransceiverFeedTwoReplicaNATS(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	for _, failure := range []string{"before-call", "after-call", "after-result"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 95*time.Second)
			defer cancel()
			cfg := transceiverNATSConfig(t, ctx)
			nodes := []*transceiverReplica{newTransceiverReplica(t, ctx, cfg), newTransceiverReplica(t, ctx, cfg)}
			first := waitTransceiverOwner(t, ctx, nodes, globalRef())
			second := nodes[0]
			if first == second {
				second = nodes[1]
			}
			var calls atomic.Int32
			var dieOnCall atomic.Bool
			var body atomic.Value
			body.Store(`[{"callsign":" ekch_twr ","transceivers":[{"frequency":119300000},{"frequency":118105000},{"frequency":118105999}]}]`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				if dieOnCall.Swap(false) {
					first.stop()
					first.nc.Close()
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body.Load().(string)))
			}))
			defer server.Close()
			provider := vatsim.NewTransceiverProvider(server.URL, time.Second, server.Client())
			feed := func(node *transceiverReplica) *TransceiverFeed {
				f, err := NewTransceiverFeed(node.nav, provider)
				require.NoError(t, err)
				return f
			}
			source := func(node *transceiverReplica) *TransceiverSource {
				s, err := NewTransceiverSource(node.nav)
				require.NoError(t, err)
				return s
			}
			// Randomized stable slot prevents a prior fixture run from sharing IDs.
			slot := time.Date(2026, 9, 30, 14, 0, 0, 0, time.UTC).Add(time.Duration(uuid.New().ID()) * time.Second)
			sent, err := feed(second).Refresh(ctx, slot)
			require.Error(t, err)
			require.False(t, sent)
			require.EqualValues(t, 0, calls.Load())
			// Concurrent scheduler passes on the accepted owner must also share
			// one persisted intent, not just sequential retries on two nodes.
			type refreshResult struct {
				sent bool
				err  error
			}
			results := make(chan refreshResult, 2)
			for i := 0; i < 2; i++ {
				go func() { sent, err := feed(first).Refresh(ctx, slot); results <- refreshResult{sent, err} }()
			}
			attempts := 0
			for i := 0; i < 2; i++ {
				result := <-results
				require.NoError(t, result.err)
				if result.sent {
					attempts++
				}
			}
			require.Equal(t, 1, attempts)
			require.EqualValues(t, 1, calls.Load())
			accepted, err := source(first).Generation(ctx)
			require.NoError(t, err)
			require.Eventually(t, func() bool { return fmt.Sprint(source(second).GetFrequencies("EKCH_TWR")) == "[118.105 119.300]" }, 5*time.Second, 20*time.Millisecond)
			// Both a duplicate invocation and nonowner retry cannot call twice.
			for _, node := range nodes {
				sent, _ = feed(node).Refresh(ctx, slot.Add(500*time.Millisecond))
				require.False(t, sent)
			}
			require.EqualValues(t, 1, calls.Load())
			for i, invalid := range []string{`{`, `[]`, `[{"callsign":"EKCH_TWR","radios":[{"frequency":125000000}]}]`} {
				body.Store(invalid)
				sent, err := feed(first).Refresh(ctx, slot.Add(time.Duration(i+1)*time.Second))
				require.Error(t, err)
				require.True(t, sent)
				generation, err := source(first).Generation(ctx)
				require.NoError(t, err)
				require.Equal(t, accepted.Revision, generation.Revision)
				require.Equal(t, accepted.Sha256, generation.Sha256)
			}
			// The accepted global revision is recoverably pending for a session
			// whose owner has never received an update callback.
			sessionID := int32(100000 + uuid.New().ID()%100000000)
			ref := sessionRef(sessionID)
			require.NoError(t, first.owner.Track(ref))
			waitTransceiverOwner(t, ctx, []*transceiverReplica{first}, ref)
			seed := &pb.Session{Id: sessionID, Airport: "EKCH", Name: "LIVE", NextStripId: 1}
			reply, err := first.nav.upsert(ctx, ref, uuid.NewString(), fmt.Sprint(sessionID), &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: seed}}, nil)
			require.NoError(t, err)
			require.Equal(t, pb.CommandOutcome_SUCCEEDED, reply.GetOutcome().GetStatus())
			require.NoError(t, second.owner.Track(ref))
			before, err := NewTransceiverSectorReconciler(source(first), first.nav.Writer, transceiverFixtureSectors)
			require.NoError(t, err)
			applied, err := before.AppliedRevision(ctx, sessionID)
			require.NoError(t, err)
			require.Zero(t, applied)
			other, err := NewTransceiverSectorReconciler(source(second), second.nav.Writer, transceiverFixtureSectors)
			require.NoError(t, err)
			require.NotEqual(t, pb.CommandOutcome_SUCCEEDED, other.Reconcile(ctx, sessionID).GetOutcome().GetStatus())

			failureSlot := slot.Add(4 * time.Second)
			failureID, err := feed(first).SlotID(failureSlot)
			require.NoError(t, err)
			body.Store(`{"transceivers":[{"callsign":"ekch_twr","frequency":120255000}]}`)
			switch failure {
			case "before-call":
				step := "external/provider/vatsim/" + transceiverResource
				derived, err := AmanIntentID(failureID, step)
				require.NoError(t, err)
				intent := &pb.WorkflowRecord{WorkflowId: failureID, Source: globalRef(), Destination: globalRef(), Step: step, DerivedCommandId: derived, Status: pb.WorkflowRecord_PENDING}
				reply, fresh := (ExternalCallWorker{Writer: first.nav.Writer}).advance(ctx, intent, "intent")
				require.True(t, fresh)
				require.Equal(t, pb.CommandOutcome_SUCCEEDED, reply.GetOutcome().GetStatus())
				first.stop()
				first.nc.Close()
			case "after-call":
				dieOnCall.Store(true)
				sent, err = feed(first).Refresh(ctx, failureSlot)
				require.True(t, sent)
				require.Error(t, err)
			case "after-result":
				first.nav.Writer.Store = &transceiverResultDeathStore{EventStore: first.nav.Writer.Store, armed: true, die: func() { first.stop(); first.nc.Close() }}
				callCtx, callCancel := context.WithTimeout(ctx, 4*time.Second)
				sent, err = feed(first).Refresh(callCtx, failureSlot)
				callCancel()
				require.True(t, sent)
				require.Error(t, err)
			}
			waitTransceiverOwner(t, ctx, []*transceiverReplica{second}, globalRef())
			waitTransceiverOwner(t, ctx, []*transceiverReplica{second}, ref)
			require.NoError(t, feed(second).Resume(ctx))
			state, err := second.nav.read(ctx, globalRef())
			require.NoError(t, err)
			if failure == "after-result" {
				require.Equal(t, pb.WorkflowRecord_COMPLETED, state.Workflows[failureID].Status)
				resultID := state.Workflows[failureID].DerivedCommandId
				require.Equal(t, state.Ledger[resultID].CommittedStreamSequence, state.Workflows[failureID].GetDestinationStreamSequence())
				require.Equal(t, []string{"120.255"}, source(second).GetFrequencies("EKCH_TWR"))
			} else {
				require.Equal(t, "CALL_UNCERTAIN", state.Workflows[failureID].ReasonCode)
				require.Equal(t, []string{"118.105", "119.300"}, source(second).GetFrequencies("EKCH_TWR"))
			}
			count := calls.Load()
			// Replay all old slots, including malformed/empty and uncertain ones.
			for i := 0; i <= 4; i++ {
				sent, err = feed(second).Refresh(ctx, slot.Add(time.Duration(i)*time.Second))
				require.NoError(t, err)
				require.False(t, sent)
			}
			require.Equal(t, count, calls.Load())
			// A transient policy failure cannot consume its generation identity.
			failing, err := NewTransceiverSectorReconciler(source(second), second.nav.Writer, func(context.Context, *pb.CommandRequest, *Aggregate, TransceiverGeneration) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
				return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("policy input unavailable")
			})
			require.NoError(t, err)
			require.Equal(t, pb.CommandReply_UNAVAILABLE, failing.Reconcile(ctx, sessionID).Status)
			applied, err = other.AppliedRevision(ctx, sessionID)
			require.NoError(t, err)
			require.Zero(t, applied)
			reconciled := other.Reconcile(ctx, sessionID)
			require.Equal(t, pb.CommandOutcome_SUCCEEDED, reconciled.GetOutcome().GetStatus(), "%v", reconciled)
			require.Equal(t, reconciled.GetStreamSequence(), other.Reconcile(ctx, sessionID).GetStreamSequence())
			generation, err := source(second).Generation(ctx)
			require.NoError(t, err)
			applied, err = other.AppliedRevision(ctx, sessionID)
			require.NoError(t, err)
			require.Equal(t, generation.Revision, applied)
			// A new persisted slot may call once and creates a pending revision.
			body.Store(`[{"callsign":"EKCH_TWR","frequency":125000000}]`)
			sent, err = feed(second).Refresh(ctx, slot.Add(5*time.Second))
			require.NoError(t, err)
			require.True(t, sent)
			require.Equal(t, count+1, calls.Load())
			generation, err = source(second).Generation(ctx)
			require.NoError(t, err)
			require.Greater(t, generation.Revision, applied)
			require.Equal(t, pb.CommandOutcome_SUCCEEDED, other.Reconcile(ctx, sessionID).GetOutcome().GetStatus())
			// Start a fresh independent backend projection. Its reader and durable
			// sector acknowledgment must reconstruct solely from NATS state.
			fresh := newTransceiverReplica(t, ctx, cfg)
			require.Equal(t, []string{"125.000"}, source(fresh).GetFrequencies(" ekch_twr "))
			readState, err := fresh.projection.Read(ref)
			require.NoError(t, err)
			require.Equal(t, generation.Revision, transceiverAppliedRevision(readState))
			for _, workflow := range readState.Workflows {
				if workflow.GetSourceRevision() == generation.Revision && workflow.Step == transceiverSectorStep+generation.Sha256 {
					require.Equal(t, pb.CommandOutcome_SUCCEEDED, readState.Ledger[workflow.DerivedCommandId].GetStatus())
				}
			}
			require.Equal(t, "125.000", readState.Indexes[pb.EntityKind_SECTOR_OWNER]["TWR"].GetValue().GetSectorOwner().Position)
			freshGeneration, err := source(fresh).Generation(ctx)
			require.NoError(t, err)
			require.Equal(t, generation.Revision, freshGeneration.Revision)
			require.Equal(t, generation.Sha256, freshGeneration.Sha256)
		})
	}
}
