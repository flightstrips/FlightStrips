package main

import (
	"bytes"
	"context"

	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The JSON below is the pinned NATS administration API and backup manifest,
// never a domain resource value. Stream entries and object/KV values remain
// exactly the original binary Protobuf bytes.
type fixtureBackupStream struct {
	Config nats.StreamConfig `json:"config"`
	State  nats.StreamState  `json:"state"`
	Data   []byte            `json:"data"`
	SHA256 string            `json:"sha256"`
}
type fixtureBackup struct {
	Streams              []fixtureBackupStream
	EffectKey, StoreKey  []byte
	BrokerConfigurations [][]byte
}

func fixtureDigest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func snapshotFixtureStream(t *testing.T, nc *nats.Conn, name string) fixtureBackupStream {
	t.Helper()
	inbox := nats.NewInbox()
	sub, err := nc.SubscribeSync(inbox)
	require.NoError(t, err)
	defer sub.Unsubscribe()
	require.NoError(t, nc.Flush())
	request, err := json.Marshal(map[string]any{"deliver_subject": inbox, "no_consumers": true, "chunk_size": 65536, "jsck": true})
	require.NoError(t, err)
	response, err := nc.Request("$JS.API.STREAM.SNAPSHOT."+name, request, 15*time.Second)
	require.NoError(t, err)
	var manifest struct {
		Config nats.StreamConfig
		State  nats.StreamState
		Error  *nats.APIError
	}
	require.NoError(t, json.Unmarshal(response.Data, &manifest))
	require.Nil(t, manifest.Error)
	var data bytes.Buffer
	for {
		message, err := sub.NextMsg(15 * time.Second)
		require.NoError(t, err)
		if len(message.Data) == 0 {
			break
		}
		_, err = data.Write(message.Data)
		require.NoError(t, err)
		if message.Reply != "" {
			require.NoError(t, message.Respond(nil))
		}
	}
	result := fixtureBackupStream{Config: manifest.Config, State: manifest.State, Data: data.Bytes(), SHA256: fixtureDigest(data.Bytes())}
	require.NotEmpty(t, result.Data)
	t.Logf("BACKUP stream=%s messages=%d first=%d last=%d bytes=%d sha256=%s", name, result.State.Msgs, result.State.FirstSeq, result.State.LastSeq, len(result.Data), result.SHA256)
	return result
}
func restoreFixtureStream(t *testing.T, nc *nats.Conn, stream fixtureBackupStream) {
	t.Helper()
	require.Equal(t, stream.SHA256, fixtureDigest(stream.Data))
	request, err := json.Marshal(struct {
		Config nats.StreamConfig `json:"config"`
		State  nats.StreamState  `json:"state"`
	}{stream.Config, stream.State})
	require.NoError(t, err)
	message, err := nc.Request("$JS.API.STREAM.RESTORE."+stream.Config.Name, request, 15*time.Second)
	require.NoError(t, err)
	var response struct {
		DeliverSubject string `json:"deliver_subject"`
		Error          *nats.APIError
	}
	require.NoError(t, json.Unmarshal(message.Data, &response))
	require.Nil(t, response.Error)
	require.NotEmpty(t, response.DeliverSubject)
	for offset := 0; offset < len(stream.Data); offset += 65536 {
		end := offset + 65536
		if end > len(stream.Data) {
			end = len(stream.Data)
		}
		_, err = nc.Request(response.DeliverSubject, stream.Data[offset:end], 15*time.Second)
		require.NoError(t, err)
	}
	message, err = nc.Request(response.DeliverSubject, nil, 30*time.Second)
	require.NoError(t, err)
	var completion struct{ Error *nats.APIError }
	require.NoError(t, json.Unmarshal(message.Data, &completion))
	require.Nil(t, completion.Error)
	js, err := nc.JetStream()
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		info, e := js.StreamInfo(stream.Config.Name)
		return e == nil && info.State.Msgs == stream.State.Msgs && info.State.LastSeq == stream.State.LastSeq && info.Config.Replicas == 3
	}, 30*time.Second, 50*time.Millisecond)
	t.Logf("RESTORE stream=%s exact_messages=%d exact_last_sequence=%d sha256=%s", stream.Config.Name, stream.State.Msgs, stream.State.LastSeq, stream.SHA256)
}

func (f *entrypointFixture) route(request *pb.CommandRequest) *pb.CommandReply {
	f.t.Helper()
	data, err := proto.Marshal(request)
	require.NoError(f.t, err)
	var result *pb.CommandReply
	f.await("typed owner command", func() bool {
		state, e := f.projection.Read(request.Aggregate)
		if e != nil || state.Owner == nil {
			return false
		}
		response, e := f.nc.Request("fs.v1.command."+state.Owner.NodeId, data, 3*time.Second)
		if e != nil {
			return false
		}
		result = &pb.CommandReply{}
		require.NoError(f.t, pb.UnmarshalStrict(response.Data, result))
		return result.Status != pb.CommandReply_NOT_OWNER && result.Status != pb.CommandReply_UNAVAILABLE
	})
	return result
}
func (f *entrypointFixture) seedEntity(ref *pb.AggregateRef, key string, value *pb.EntityRecord) {
	f.t.Helper()
	zero := uint64(0)
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "task22-seed"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: key, Value: value}}}}}
	result := f.route(request)
	require.Equal(f.t, pb.CommandOutcome_SUCCEEDED, result.GetOutcome().GetStatus(), result)
	f.t.Logf("SEED id=%s sequence=%d key=%s", request.CommandId, result.GetStreamSequence(), key)
}

func TestServerNATSEncryptedBackupRestore(t *testing.T) {
	if os.Getenv("NATS_TASK22") != "1" {
		t.Skip("requires explicit disposable Task22 native fixture")
	}
	f := newEntrypointFixture(t, true)
	name, ref, plugins := f.seededSession()
	front := f.front(1, name)
	command := uuid.NewString()
	sendEntrypointFrame(t, front, privateCommand(command))
	f.await("encrypted effect delivered", func() bool { return len(plugins[0].effect(command)) == 1 })
	// Keep the pending claim and its deadline in the backup. Restoring it must
	// never dispatch it again, irrespective of fresh client presence.
	airport := &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: "EKCH"}}}
	now := timestamppb.Now()
	auditID := uuid.NewString()
	f.seedEntity(airport, auditID, &pb.EntityRecord{Value: &pb.EntityRecord_AmanAudit{AmanAudit: &pb.AmanAudit{Id: auditID, AirportRevision: 1, CreatedAt: now, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "task22-seed"}, Fact: &pb.AmanAudit_Replay{Replay: &pb.AmanReplayAudit{Source: "task22", RecordId: command, Result: "restored"}}}}})
	deadlineID := "task22-pending"
	f.seedEntity(ref, deadlineID, &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: &pb.SessionDeadline{Id: deadlineID, Kind: "session-update", DueAt: timestamppb.New(time.Now().Add(time.Hour)), CommandId: uuid.NewString(), SourceRevision: 1}}})
	objects, err := f.projection.JS.ObjectStore(f.resources.Names.Objects)
	require.NoError(t, err)
	nav := &pb.ObjectValue{SchemaVersion: 1, Content: &pb.ObjectValue_Nav{Nav: &pb.NavData{Airport: "EKCH", SchemaVersion: "1", Version: &pb.NavDatasetVersion{Cycle: "2610"}, ValidationState: "candidate", ImportedAt: now, Fragment: &pb.NavData_AirportFragment{AirportFragment: &pb.NavAirportFragment{Airport: &pb.NavAirport{Icao: "EKCH", Name: "Task22 restore fixture", Position: &pb.NavCoordinate{LatitudeDegrees: 55.618, LongitudeDegrees: 12.656}}}}}}}
	navBytes, err := proto.Marshal(nav)
	require.NoError(t, err)
	navDigest := fixtureDigest(navBytes)
	navName := "nav/" + navDigest
	_, err = objects.PutBytes(navName, navBytes)
	require.NoError(t, err)
	f.seedEntity(airport, "EKCH", &pb.EntityRecord{Value: &pb.EntityRecord_NavManifest{NavManifest: &pb.NavManifest{Airport: "EKCH", Cycle: "2610", Digest: navDigest, Objects: []*pb.NavObjectRef{{Kind: "airport", ObjectName: navName, Sha256: navDigest}}, Active: true}}})
	position := &pb.PositionValue{SchemaVersion: 1, SessionId: ref.GetSession().Id, AircraftKey: "SAS123", OwnerEpoch: f.state(ref).Owner.Epoch, ObservedAt: now, Observation: &pb.PositionValue_Position{Position: &pb.AircraftPosition{Latitude: 55.618, Longitude: 12.656, AltitudeFeet: 10000}}}
	positionData, err := proto.Marshal(position)
	require.NoError(t, err)
	positionKey := fmt.Sprintf("%d.SAS123.%d", position.SessionId, position.OwnerEpoch)
	positionRevision, err := f.projection.Positions.Put(positionKey, positionData)
	require.NoError(t, err)
	for _, p := range f.apps {
		p.stop()
	}
	info, err := f.projection.JS.StreamInfo(f.resources.Names.State)
	require.NoError(t, err)
	require.NoError(t, f.projection.WaitApplied(f.ctx, info.State.LastSeq))
	refs := []*pb.AggregateRef{{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}, airport, ref}
	before := map[string]*pb.Snapshot{}
	for _, aggregate := range refs {
		state := f.state(aggregate)
		require.NoError(t, f.projection.Snapshots.Save(state))
		snapshot, e := state.Snapshot()
		require.NoError(t, e)
		subject, e := cluster.Subject(aggregate)
		require.NoError(t, e)
		before[subject] = snapshot
		require.NotEmpty(t, snapshot.Entities)
		t.Logf("CHECKPOINT aggregate=%s entity_count=%d revision=%d sequence=%d sha256=%s", subject, len(snapshot.Entities), snapshot.AggregateRevision, snapshot.LastStreamSequence, snapshot.Sha256)
	}
	adminCfg := f.resources
	for i, u := range adminCfg.URLs {
		adminCfg.URLs[i] = strings.ReplaceAll(u, "backend:backend-local-only", "bootstrap:bootstrap-local-only")
	}
	admin, err := natsresources.Connect(adminCfg)
	require.NoError(t, err)
	defer admin.Close()
	backup := fixtureBackup{}
	backup.EffectKey, err = os.ReadFile(filepath.Join(f.dir, "effects.key"))
	require.NoError(t, err)
	backup.StoreKey, err = os.ReadFile(filepath.Join(f.dir, "store.key"))
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		data, e := os.ReadFile(filepath.Join(f.dir, fmt.Sprintf("nats-%d.conf", i)))
		require.NoError(t, e)
		backup.BrokerConfigurations = append(backup.BrokerConfigurations, data)
	}
	for _, stream := range []string{"FS_STATE", "KV_FS_POSITIONS", "KV_FS_SNAPSHOT_INDEX", "OBJ_FS_OBJECTS"} {
		backup.Streams = append(backup.Streams, snapshotFixtureStream(t, admin, stream))
	}
	plain, err := json.Marshal(backup)
	require.NoError(t, err)
	backupKey := make([]byte, 32)
	_, err = rand.Read(backupKey)
	require.NoError(t, err)
	block, err := aes.NewCipher(backupKey)
	require.NoError(t, err)
	aead, err := cipher.NewGCM(block)
	require.NoError(t, err)
	nonce := make([]byte, aead.NonceSize())
	_, err = rand.Read(nonce)
	require.NoError(t, err)
	encrypted := aead.Seal(nonce, nonce, plain, []byte("task22-backup-v1"))
	backupDir := t.TempDir()
	backupPath := filepath.Join(backupDir, "off-cluster-backup.aesgcm")
	require.NoError(t, os.WriteFile(backupPath, encrypted, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(backupDir, "backup.key"), backupKey, 0600))
	t.Logf("ENCRYPTED_BACKUP sha256=%s bytes=%d source_high_water=%d", fixtureDigest(encrypted), len(encrypted), info.State.LastSeq)
	corrupt := append([]byte(nil), encrypted...)
	corrupt[len(corrupt)-1] ^= 1
	_, err = aead.Open(nil, corrupt[:aead.NonceSize()], corrupt[aead.NonceSize():], []byte("task22-backup-v1"))
	require.Error(t, err, "authenticated backup corruption must fail")
	// Stop the source cluster completely. Restore uses only off-cluster backup
	// bytes and restored credentials/keys; it cannot query the original cluster.
	for _, p := range f.brokers {
		p.stop()
	}
	backupData, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	plain, err = aead.Open(nil, backupData[:aead.NonceSize()], backupData[aead.NonceSize():], []byte("task22-backup-v1"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(plain, &backup))
	restoreDir := t.TempDir()
	clients, routes := []string{}, []string{}
	for i := 0; i < 3; i++ {
		clients = append(clients, entrypointAddress(t))
		routes = append(routes, entrypointAddress(t))
	}
	restoreURLs := []string{}
	for i := 0; i < 3; i++ {
		conf := string(backup.BrokerConfigurations[i])
		oldConfig := conf
		// Rewrite only listen/routes/store/cluster fixture identities. The complete
		// authorization allowlist and AES store encryption configuration are restored.
		for line := range strings.SplitSeq(oldConfig, "\n") {
			trim := strings.TrimSpace(line)
			if strings.HasPrefix(trim, "listen:") {
				if strings.HasPrefix(line, "  ") {
					conf = strings.Replace(conf, line, "  listen: "+routes[i], 1)
				} else {
					conf = strings.Replace(conf, line, "listen: "+clients[i], 1)
				}
			}
		}
		conf = strings.ReplaceAll(conf, filepath.ToSlash(filepath.Join(f.dir, fmt.Sprintf("data-%d", i))), filepath.ToSlash(filepath.Join(restoreDir, fmt.Sprintf("data-%d", i))))
		conf = strings.ReplaceAll(conf, "task22-"+fixtureDigest([]byte(f.dir))[:12], "task22-restore-"+fixtureDigest([]byte(restoreDir))[:12])
		// Each route URL comes from a source config; substitute all peers by their
		// original cluster listen address gathered from the backed-up configurations.
		for peer, peerConfig := range backup.BrokerConfigurations {
			for line := range strings.SplitSeq(string(peerConfig), "\n") {
				if strings.HasPrefix(line, "  listen:") {
					old := strings.TrimSpace(strings.TrimPrefix(line, "  listen:"))
					conf = strings.ReplaceAll(conf, "nats://"+old, "nats://"+routes[peer])
				}
			}
		}
		path := filepath.Join(restoreDir, fmt.Sprintf("nats-%d.conf", i))
		require.NoError(t, os.WriteFile(path, []byte(conf), 0600))
		startFixtureProcess(t, os.Getenv("NATS_SERVER_BINARY"), f.backend, fixtureEnv(map[string]string{"TASK22_STORE_KEY": "task22-" + hex.EncodeToString(backup.StoreKey)}), "-c", path)
		restoreURLs = append(restoreURLs, "nats://bootstrap:bootstrap-local-only@"+clients[i])
	}
	restoredCfg := f.resources
	restoredCfg.URLs = restoreURLs
	var restoredAdmin *nats.Conn
	require.Eventually(t, func() bool {
		restoredAdmin, err = natsresources.Connect(restoredCfg)
		if err != nil {
			return false
		}
		if natscluster.WaitForQuorum(f.ctx, restoredAdmin) != nil {
			restoredAdmin.Close()
			return false
		}
		return true
	}, 30*time.Second, 100*time.Millisecond)
	defer restoredAdmin.Close()
	for _, stream := range backup.Streams {
		restoreFixtureStream(t, restoredAdmin, stream)
	}
	// Bootstrap creates only disposable presence; verifies restored durable config.
	require.NoError(t, natsresources.Bootstrap(f.ctx, restoredAdmin, restoredCfg))
	for i, u := range restoreURLs {
		restoreURLs[i] = strings.ReplaceAll(u, "bootstrap:bootstrap-local-only", "backend:backend-local-only")
	}
	restoredCfg.URLs = restoreURLs
	restoredNC, err := natsresources.Connect(restoredCfg)
	require.NoError(t, err)
	defer restoredNC.Close()
	projection, err := cluster.NewProjection(restoredNC, restoredCfg)
	require.NoError(t, err)
	projectionCtx, stopProjection := context.WithCancel(f.ctx)
	defer stopProjection()
	done := make(chan error, 1)
	go func() { done <- projection.Run(projectionCtx) }()
	defer func() { stopProjection(); <-done }()
	require.Eventually(t, func() bool { return projection.Ready() == nil }, 30*time.Second, 50*time.Millisecond)
	for _, aggregate := range refs {
		subject, e := cluster.Subject(aggregate)
		require.NoError(t, e)
		state, e := projection.Read(aggregate)
		require.NoError(t, e)
		snapshot, e := state.Snapshot()
		require.NoError(t, e)
		require.True(t, proto.Equal(before[subject], snapshot), "exact entities/revisions/outcomes/workflows/effects/audits/nav/deadlines at "+subject)
		t.Logf("RESTORE_COMPARE aggregate=%s entities=%d revision=%d sequence=%d sha256=%s exact=true", subject, len(snapshot.Entities), snapshot.AggregateRevision, snapshot.LastStreamSequence, snapshot.Sha256)
	}
	positionEntry, err := projection.Positions.Get(positionKey)
	require.NoError(t, err)
	require.Equal(t, positionRevision, positionEntry.Revision())
	require.Equal(t, positionData, positionEntry.Value())
	restoredObjects, err := projection.JS.ObjectStore(restoredCfg.Names.Objects)
	require.NoError(t, err)
	navRestored, err := restoredObjects.GetBytes(navName)
	require.NoError(t, err)
	require.Equal(t, navDigest, fixtureDigest(navRestored))
	restoredKey := filepath.Join(restoreDir, "effects.key")
	require.NoError(t, os.WriteFile(restoredKey, backup.EffectKey, 0600))
	secrets, err := cluster.LoadEffectSecrets(restoredObjects, "v1", map[string]string{"v1": restoredKey})
	require.NoError(t, err)
	effect := frozenEffect(before, ref, command)
	require.NotNil(t, effect)
	require.Equal(t, pb.EffectRecord_DISPATCH_CLAIMED, effect.Status)
	body, err := secrets.OpenPrivateMessage(command, effect.TargetCid, effect.GetPrivateMessage().ObjectName, effect.GetPrivateMessage().Sha256)
	require.NoError(t, err)
	require.Equal(t, "local fault fixture", body)
	// Start two real servers against the entirely separate restored cluster.
	env := replaceFixtureEnv(f.env, map[string]string{"NATS_URLS": strings.Join(restoreURLs, ","), "NATS_EFFECT_KEY_FILES": "v1=" + restoredKey, "TASK22_GATE_DIR": ""})
	addresses := []string{entrypointAddress(t), entrypointAddress(t)}
	for _, address := range addresses {
		startFixtureProcess(t, f.binary, f.backend, env, "-addr", address)
	}
	require.Eventually(t, func() bool {
		a, _ := entrypointStatus(addresses[0], "/readyz", "")
		b, _ := entrypointStatus(addresses[1], "/readyz", "")
		return a == 200 && b == 200
	}, 30*time.Second, 50*time.Millisecond)
	restoredFixture := &entrypointFixture{t: t, ctx: f.ctx, addresses: addresses, token: f.token, projection: projection}
	reconnected := restoredFixture.plugin(0, name, "111111")
	restoredFixture.await("restored claimed effect becomes visibly unknown", func() bool {
		return restoredFixture.outcome(0, command) == "unknown" && restoredFixture.outcome(1, command) == "unknown"
	})
	require.Empty(t, reconnected.effect(command), "restored claim must never be automatically resent to the reconnected CID")
	require.NotNil(t, restoredFixture.state(ref).Indexes[pb.EntityKind_SESSION_DEADLINE][deadlineID])
	t.Logf("RESTORE_KEYS credentials=true aes_store=true effect_decryption=true nav_sha256=%s position_revision=%d pending_deadline=%s", navDigest, positionRevision, deadlineID)
}

func replaceFixtureEnv(env []string, values map[string]string) []string {
	result := []string{}
	for _, item := range env {
		key, _, _ := strings.Cut(item, "=")
		if _, replace := values[key]; !replace {
			result = append(result, item)
		}
	}
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}
func frozenEffect(snapshots map[string]*pb.Snapshot, ref *pb.AggregateRef, id string) *pb.EffectRecord {
	subject, _ := cluster.Subject(ref)
	for _, effect := range snapshots[subject].Effects {
		if effect.CommandId == id {
			return effect
		}
	}
	return nil
}
