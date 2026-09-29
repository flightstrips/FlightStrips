package natsresources

import (
	"FlightStrips/internal/testing/natscluster"
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// Run with NATS_INTEGRATION=1 and docker compose -f docker-compose.nats.yaml up -d.
func TestClusterResources(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires three-node NATS fixture")
	}
	admin := Config{URLs: []string{"nats://bootstrap:bootstrap-local-only@127.0.0.1:4222", "nats://bootstrap:bootstrap-local-only@127.0.0.1:4223", "nats://bootstrap:bootstrap-local-only@127.0.0.1:4224"}, ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: RequiredNames}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	nc, err := Connect(admin)
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	if err := natscluster.WaitForQuorum(ctx, nc); err != nil {
		t.Fatal(err)
	}
	if err := Bootstrap(ctx, nc, admin); err != nil {
		t.Fatal(err)
	}
	js, err := nc.JetStream(nats.MaxWait(admin.RequestTimeout))
	if err != nil {
		t.Fatal(err)
	}
	before := make(map[string]nats.StreamConfig)
	for _, name := range []string{"FS_STATE", "KV_FS_POSITIONS", "KV_FS_PRESENCE", "KV_FS_SNAPSHOT_INDEX", "OBJ_FS_OBJECTS"} {
		info, err := js.StreamInfo(name)
		if err != nil {
			t.Fatal(err)
		}
		before[name] = info.Config
	}
	if err := Bootstrap(ctx, nc, admin); err != nil {
		t.Fatal(err)
	}
	for name, original := range before {
		info, err := js.StreamInfo(name)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(info.Config, original) {
			t.Fatalf("second bootstrap changed %s", name)
		}
	}
	app := admin
	app.URLs = []string{"nats://backend:backend-local-only@127.0.0.1:4222"}
	appNC, err := Connect(app)
	if err != nil {
		t.Fatal(err)
	}
	defer appNC.Close()
	if err := Verify(ctx, appNC, app); err != nil {
		t.Fatalf("application verification: %v", err)
	}
	appJS, err := appNC.JetStream(nats.MaxWait(app.RequestTimeout))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := appJS.AddStream(&nats.StreamConfig{Name: "FS_APP_MUST_NOT_CREATE", Subjects: []string{"fs.app.must.not.create"}}); err == nil {
		t.Fatal("application credential unexpectedly created a stream")
	}
	for _, tc := range []struct {
		name   string
		change func(*nats.StreamConfig)
	}{
		{"FS_STATE", func(c *nats.StreamConfig) { c.Subjects = []string{"fs.v1.state.global"} }},
		{"FS_STATE", func(c *nats.StreamConfig) { c.Replicas = 2 }},
		{"FS_STATE", func(c *nats.StreamConfig) { c.Retention = nats.InterestPolicy }},
		{"FS_STATE", func(c *nats.StreamConfig) { c.MaxAge = time.Hour }},
		{"KV_FS_POSITIONS", func(c *nats.StreamConfig) { c.Storage = nats.MemoryStorage }},
		{"KV_FS_PRESENCE", func(c *nats.StreamConfig) { c.MaxAge = time.Minute }},
		{"KV_FS_SNAPSHOT_INDEX", func(c *nats.StreamConfig) { c.MaxMsgsPerSubject = 1 }},
		{"OBJ_FS_OBJECTS", func(c *nats.StreamConfig) { c.Replicas = 2 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info, err := js.StreamInfo(tc.name)
			if err != nil {
				t.Fatal(err)
			}
			original := info.Config
			changed := original
			tc.change(&changed)
			recreate := tc.name == "KV_FS_POSITIONS"
			if recreate {
				if err := js.DeleteStream(tc.name); err != nil {
					t.Fatal(err)
				}
				if _, err := js.AddStream(&changed); err != nil {
					t.Fatal(err)
				}
			} else if _, err := js.UpdateStream(&changed); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if recreate {
					if err := js.DeleteStream(tc.name); err != nil {
						t.Errorf("delete drifted %s: %v", tc.name, err)
						return
					}
					if _, err := js.AddStream(&original); err != nil {
						t.Errorf("restore %s: %v", tc.name, err)
					}
				} else if _, err := js.UpdateStream(&original); err != nil {
					t.Errorf("restore %s: %v", tc.name, err)
				}
			}()
			if err := Verify(ctx, nc, admin); err == nil || !strings.Contains(err.Error(), tc.name) {
				t.Fatalf("expected named drift for %s, got %v", tc.name, err)
			}
		})
	}
	if err := Verify(ctx, nc, admin); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareWriteProbe(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires three-node NATS fixture")
	}
	nc, err := nats.Connect("nats://bootstrap:bootstrap-local-only@127.0.0.1:4222")
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := nc.JetStream(nats.MaxWait(2 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := js.StreamInfo("FS_WRITE_PROBE"); err == nil {
		return
	}
	if _, err := js.AddStream(&nats.StreamConfig{Name: "FS_WRITE_PROBE", Subjects: []string{"fs.write.probe"}, Storage: nats.FileStorage, Replicas: 3}); err != nil {
		t.Fatal(err)
	}
}

func TestDurableWrite(t *testing.T) {
	if os.Getenv("NATS_EXPECT_WRITE") == "" {
		t.Skip("requires an explicit quorum expectation")
	}
	nc, err := nats.Connect("nats://bootstrap:bootstrap-local-only@127.0.0.1:4222")
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := nc.JetStream(nats.MaxWait(2 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	ack, err := js.Publish("fs.write.probe", []byte("durable probe"))
	if os.Getenv("NATS_EXPECT_WRITE") == "1" {
		if err != nil || ack.Stream != "FS_WRITE_PROBE" {
			t.Fatalf("expected durable PubAck, got %v, %v", ack, err)
		}
	} else if err == nil {
		t.Fatalf("unexpected durable PubAck without quorum: %v", ack)
	}
}
