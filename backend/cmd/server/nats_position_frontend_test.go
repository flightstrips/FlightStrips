package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/frontendbinary"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type loadFrontendAttempt struct {
	ID       string                   `json:"command_id"`
	Revision uint64                   `json:"expected_entity_revision"`
	Status   pb.CommandOutcome_Status `json:"status"`
	Reason   string                   `json:"reason"`
}

type loadFrontendAction struct {
	LogicalID string                `json:"logical_id"`
	Attempts  []loadFrontendAttempt `json:"attempts"`
	Err       string                `json:"error,omitempty"`
}

// The control strip is still observed by VATSIM. Use the revision delivered to
// this real client and retry only a proved optimistic-concurrency conflict.
// Each scheduled action runs independently of the surveillance sender.
type loadFrontend struct {
	conn       *websocket.Conn
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	writeMu    sync.Mutex
	revision   uint64
	changed    chan struct{}
	waiters    map[string]chan *pb.FrontendActionResult
	actions    []*loadFrontendAction
	jobs       sync.WaitGroup
	readerDone chan struct{}
	readerErr  error
}

func (f *entrypointFixture) loadFrontend(node int, name string) *loadFrontend {
	f.t.Helper()
	c := f.dial(node, "/frontEndEvents", frontendbinary.Subprotocol)
	sendEntrypointFrame(f.t, c, &pb.FrontendFrame{ProtocolRevision: 2, Frame: &pb.FrontendFrame_Authenticate{Authenticate: &pb.FrontendAuthenticate{BearerToken: f.token, Airport: "EKCH", SessionName: name}}})
	_ = c.SetReadDeadline(time.Now().Add(15 * time.Second))
	kind, data, err := c.ReadMessage()
	require.NoError(f.t, err)
	require.Equal(f.t, websocket.BinaryMessage, kind)
	initial := &pb.FrontendFrame{}
	require.NoError(f.t, pb.UnmarshalStrict(data, initial))
	require.NotNil(f.t, initial.GetInitial())
	ctx, cancel := context.WithCancel(f.ctx)
	client := &loadFrontend{conn: c, ctx: ctx, cancel: cancel, changed: make(chan struct{}), waiters: map[string]chan *pb.FrontendActionResult{}, readerDone: make(chan struct{})}
	for _, entity := range initial.GetInitial().Entities {
		if entity.Key == "SAS199" && entity.GetValue().GetStrip() != nil {
			client.revision = entity.Revision
		}
	}
	require.NotZero(f.t, client.revision)
	_ = c.SetReadDeadline(time.Time{})
	go func() {
		defer close(client.readerDone)
		defer cancel()
		last := map[string]uint64{"session": initial.GetInitial().AggregateRevision, "airport": initial.GetInitial().AirportAggregateRevision}
		for {
			kind, data, err := c.ReadMessage()
			if err != nil {
				if ctx.Err() == nil {
					client.mu.Lock()
					client.readerErr = fmt.Errorf("frontend node %d reader: %w", node, err)
					revision := client.revision
					client.mu.Unlock()
					f.t.Logf("FRONTEND_READER_FAILURE node=%d time=%s entity_revision=%d session_revision=%d airport_revision=%d error=%v", node, time.Now().UTC().Format(time.RFC3339Nano), revision, last["session"], last["airport"], err)
				}
				return
			}
			frame := &pb.FrontendFrame{}
			if kind != websocket.BinaryMessage || pb.UnmarshalStrict(data, frame) != nil {
				f.t.Error("invalid binary frontend frame")
				return
			}
			if delta := frame.GetDelta(); delta != nil {
				key := "session"
				if delta.Aggregate.GetAirport() != nil {
					key = "airport"
				}
				if delta.AggregateRevision <= last[key] {
					f.t.Errorf("snapshot/delta ordering: %s %d <= %d", key, delta.AggregateRevision, last[key])
					return
				}
				last[key] = delta.AggregateRevision
				if key == "session" {
					client.mu.Lock()
					for _, change := range delta.Changes {
						if change.Key == "SAS199" && change.GetUpsert().GetStrip() != nil && change.Revision > client.revision {
							client.revision = change.Revision
							close(client.changed)
							client.changed = make(chan struct{})
						}
					}
					client.mu.Unlock()
				}
			}
			if result := frame.GetActionResult(); result != nil {
				f.t.Logf("FRONTEND_RESULT id=%s status=%s reason=%s detail=%s", result.RequestId, result.Status, result.ReasonCode, result.Detail)
				client.mu.Lock()
				waiter := client.waiters[result.RequestId]
				if waiter != nil {
					delete(client.waiters, result.RequestId)
					waiter <- result
				}
				client.mu.Unlock()
			}
		}
	}()
	f.t.Cleanup(func() { cancel(); _ = c.Close(); client.jobs.Wait(); <-client.readerDone })
	return client
}

func (c *loadFrontend) currentRevision(ctx context.Context, newerThan uint64) (uint64, error) {
	for {
		c.mu.Lock()
		revision, changed := c.revision, c.changed
		c.mu.Unlock()
		if revision > newerThan {
			return revision, nil
		}
		select {
		case <-ctx.Done():
			return 0, c.actionError(ctx.Err())
		case <-changed:
		}
	}
}

// Preserve the original connection failure instead of reducing every later
// scheduled action to an unexplained context cancellation. This never retries
// a closed socket or changes the final durable-success assertion.
func (c *loadFrontend) actionError(err error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.readerErr != nil {
		return fmt.Errorf("%w: %v", err, c.readerErr)
	}
	return err
}

func (c *loadFrontend) marked(logicalID string, value bool) {
	action := &loadFrontendAction{LogicalID: logicalID}
	c.mu.Lock()
	c.actions = append(c.actions, action)
	c.mu.Unlock()
	c.jobs.Add(1)
	go func() {
		defer c.jobs.Done()
		ctx, cancel := context.WithTimeout(c.ctx, 3*time.Second)
		defer cancel()
		var prior uint64
		for attempt := 0; attempt < 4; attempt++ {
			revision, err := c.currentRevision(ctx, prior)
			if err != nil {
				action.Err = err.Error()
				return
			}
			id := logicalID
			if attempt > 0 {
				id = uuid.NewString()
			}
			record := loadFrontendAttempt{ID: id, Revision: revision}
			waiter := make(chan *pb.FrontendActionResult, 1)
			c.mu.Lock()
			c.waiters[id] = waiter
			c.mu.Unlock()
			action.Attempts = append(action.Attempts, record)
			data, err := proto.Marshal(markedCommand(id, "SAS199", revision, value))
			if err == nil {
				c.writeMu.Lock()
				if err = ctx.Err(); err == nil {
					deadline, _ := ctx.Deadline()
					_ = c.conn.SetWriteDeadline(deadline)
					err = c.conn.WriteMessage(websocket.BinaryMessage, data)
				}
				c.writeMu.Unlock()
			}
			if err != nil {
				action.Err = c.actionError(err).Error()
				c.mu.Lock()
				delete(c.waiters, id)
				c.mu.Unlock()
				return
			}
			var result *pb.FrontendActionResult
			select {
			case result = <-waiter:
			case <-ctx.Done():
				action.Err = c.actionError(ctx.Err()).Error()
				c.mu.Lock()
				delete(c.waiters, id)
				c.mu.Unlock()
				return
			}
			action.Attempts[len(action.Attempts)-1].Status = result.Status
			action.Attempts[len(action.Attempts)-1].Reason = result.ReasonCode
			if result.Status == pb.CommandOutcome_SUCCEEDED {
				return
			}
			if result.Status != pb.CommandOutcome_FAILED || result.ReasonCode != "REVISION_CONFLICT" {
				action.Err = fmt.Sprintf("unexpected result %s/%s", result.Status, result.ReasonCode)
				return
			}
			prior = revision
		}
		action.Err = "revision conflict retry limit exhausted"
	}()
}

func (c *loadFrontend) finish() []*loadFrontendAction {
	c.jobs.Wait()
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*loadFrontendAction(nil), c.actions...)
}

func TestLoadFrontendWaitsForNewRevisionAndDoesNotRetryOtherFailures(t *testing.T) {
	for _, reason := range []string{"REVISION_CONFLICT", "UNAUTHORIZED", "CONNECTION_CLOSED"} {
		t.Run(reason, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			ref := sessionFaultRef(42)
			connected := make(chan *websocket.Conn, 1)
			commands := make(chan *pb.FrontendCommand, 4)
			errors := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := (&websocket.Upgrader{Subprotocols: []string{frontendbinary.Subprotocol}}).Upgrade(w, r, nil)
				if err != nil {
					errors <- err
					return
				}
				defer conn.Close()
				if _, _, err = conn.ReadMessage(); err != nil {
					errors <- err
					return
				}
				initial := &pb.FrontendFrame{ProtocolRevision: 2, Frame: &pb.FrontendFrame_Initial{Initial: &pb.FrontendInitial{SessionId: 42, AggregateRevision: 1, Entities: []*pb.EntitySnapshot{{Key: "SAS199", Revision: 7, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Callsign: "SAS199"}}}}}}}}
				data, err := proto.Marshal(initial)
				if err == nil {
					err = conn.WriteMessage(websocket.BinaryMessage, data)
				}
				if err != nil {
					errors <- err
					return
				}
				connected <- conn
				for {
					_, data, err := conn.ReadMessage()
					if err != nil {
						return
					}
					frame := &pb.FrontendFrame{}
					if err = pb.UnmarshalStrict(data, frame); err != nil {
						errors <- err
						return
					}
					select {
					case commands <- frame.GetCommand():
					case <-ctx.Done():
						return
					}
				}
			}))
			t.Cleanup(server.Close)
			f := &entrypointFixture{t: t, ctx: ctx, addresses: []string{strings.TrimPrefix(server.URL, "http://")}}
			client := f.loadFrontend(0, "LIVE")
			var conn *websocket.Conn
			select {
			case conn = <-connected:
			case err := <-errors:
				t.Fatal(err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			send := func(frame *pb.FrontendFrame) {
				data, err := proto.Marshal(frame)
				require.NoError(t, err)
				require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, data))
			}
			client.marked("original", true)
			var first *pb.FrontendCommand
			select {
			case first = <-commands:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			require.Equal(t, uint64(7), first.GetExpectedEntityRevision())
			resultReason := reason
			if reason == "CONNECTION_CLOSED" {
				resultReason = "REVISION_CONFLICT"
			}
			send(&pb.FrontendFrame{ProtocolRevision: 2, Frame: &pb.FrontendFrame_ActionResult{ActionResult: &pb.FrontendActionResult{RequestId: first.RequestId, Status: pb.CommandOutcome_FAILED, ReasonCode: resultReason}}})
			select {
			case command := <-commands:
				t.Fatalf("retried without a strictly newer revision: %v", command)
			case <-time.After(20 * time.Millisecond):
			}
			if reason == "CONNECTION_CLOSED" {
				require.NoError(t, conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseTryAgainLater, "projection unavailable"), time.Now().Add(time.Second)))
				select {
				case <-client.readerDone:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			if reason == "REVISION_CONFLICT" {
				send(&pb.FrontendFrame{ProtocolRevision: 2, Frame: &pb.FrontendFrame_Delta{Delta: &pb.FrontendDelta{Aggregate: ref, AggregateRevision: 2, Changes: []*pb.EntityChange{{Key: "SAS199", Revision: 8, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Callsign: "SAS199"}}}}}}}}})
				var retry *pb.FrontendCommand
				select {
				case retry = <-commands:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				require.NotEqual(t, first.RequestId, retry.RequestId)
				require.Equal(t, uint64(8), retry.GetExpectedEntityRevision())
				require.True(t, retry.GetAction().GetStrip().GetSetMarked().Marked)
				send(&pb.FrontendFrame{ProtocolRevision: 2, Frame: &pb.FrontendFrame_ActionResult{ActionResult: &pb.FrontendActionResult{RequestId: retry.RequestId, Status: pb.CommandOutcome_SUCCEEDED}}})
			}
			actions := client.finish()
			require.Len(t, actions, 1)
			if reason == "REVISION_CONFLICT" {
				require.Empty(t, actions[0].Err)
				require.Len(t, actions[0].Attempts, 2)
			} else {
				require.NotEmpty(t, actions[0].Err)
				require.Len(t, actions[0].Attempts, 1)
				if reason == "CONNECTION_CLOSED" {
					require.Contains(t, actions[0].Err, "1013")
					require.Contains(t, actions[0].Err, "projection unavailable")
					require.Contains(t, actions[0].Err, "frontend node 0")
				}
			}
		})
	}
}
