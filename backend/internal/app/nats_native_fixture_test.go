package app

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type nativeRuntimeBrokerLog struct {
	sync.Mutex
	buffer bytes.Buffer
}

func (l *nativeRuntimeBrokerLog) Write(p []byte) (int, error) {
	l.Lock()
	defer l.Unlock()
	return l.buffer.Write(p)
}

func (l *nativeRuntimeBrokerLog) text() string {
	l.Lock()
	defer l.Unlock()
	return l.buffer.String()
}

type nativeRuntimeBroker struct {
	command *exec.Cmd
	done    chan struct{}
	err     error
	log     *nativeRuntimeBrokerLog
}

type nativeRuntimeBrokers struct {
	t       *testing.T
	binary  string
	base    int
	env     []string
	configs [3]string
	nodes   [3]*nativeRuntimeBroker
}

// Every process and persistent store belongs to this single test. The native
// path never stops the brokers supplied by an outer integration runner.
func newNativeRuntimeBrokers(t *testing.T, binary string) *nativeRuntimeBrokers {
	t.Helper()
	binary, err := filepath.Abs(binary)
	require.NoError(t, err)
	version, err := exec.Command(binary, "--version").CombinedOutput()
	require.NoError(t, err, string(version))
	require.Equal(t, "nats-server: v2.15.0", strings.TrimSpace(string(version)))
	compiled, err := os.ReadFile(binary)
	require.NoError(t, err)
	digest := sha256.Sum256(compiled)
	t.Logf("NATIVE_QUORUM_BROKER binary=%s version=%s sha256=%x", binary, strings.TrimSpace(string(version)), digest)
	var listeners []net.Listener
	base := 0
	for attempt := 0; attempt < 100; attempt++ {
		value, err := rand.Int(rand.Reader, big.NewInt(40000))
		require.NoError(t, err)
		base = 10000 + int(value.Int64())
		for offset := 0; offset < 6; offset++ {
			listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", base+offset))
			if err != nil {
				break
			}
			listeners = append(listeners, listener)
		}
		if len(listeners) == 6 {
			break
		}
		for _, listener := range listeners {
			_ = listener.Close()
		}
		listeners = nil
	}
	require.Len(t, listeners, 6, "reserve six private client/route ports")
	defer func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}()
	key := make([]byte, 32)
	_, err = rand.Read(key)
	require.NoError(t, err)
	env := []string{}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "TASK22_STORE_KEY=") {
			env = append(env, entry)
		}
	}
	env = append(env, "TASK22_STORE_KEY="+hex.EncodeToString(key))
	fixture := &nativeRuntimeBrokers{t: t, binary: binary, base: base, env: env}
	dir := t.TempDir()
	// Register before the runtime fixture, so applications join all workers
	// and flush state before these brokers are stopped during cleanup.
	t.Cleanup(func() {
		for node := range fixture.nodes {
			fixture.stop(node)
		}
	})
	template, err := os.ReadFile(filepath.Join("..", "..", "testdata", "nats", "nats-1.conf"))
	require.NoError(t, err)
	for node := range fixture.nodes {
		store := filepath.Join(dir, fmt.Sprintf("store-%d", node))
		require.NoError(t, os.Mkdir(store, 0700))
		routes := []string{}
		for peer := range fixture.nodes {
			if peer != node {
				routes = append(routes, fmt.Sprintf("\"nats://127.0.0.1:%d\"", base+3+peer))
			}
		}
		config := strings.NewReplacer(
			"server_name: nats-1", fmt.Sprintf("server_name: native-quorum-%d-%d", base, node),
			"port: 4222", fmt.Sprintf("host: 127.0.0.1\nport: %d", base+node),
			"name: flightstrips-local", fmt.Sprintf("name: native-quorum-%d", base),
			"port: 6222", fmt.Sprintf("host: 127.0.0.1\n  port: %d", base+3+node),
			"\"/data/jetstream\"", strconv.Quote(filepath.ToSlash(store)),
			"jetstream {", "jetstream { max_memory_store: 64MB, max_file_store: 1GB, cipher: aes, key: $TASK22_STORE_KEY,",
			"routes: [\"nats://nats-2:6222\", \"nats://nats-3:6222\"]", "routes: ["+strings.Join(routes, ", ")+"]",
		).Replace(string(template))
		fixture.configs[node] = filepath.Join(dir, fmt.Sprintf("node-%d.conf", node))
		require.NoError(t, os.WriteFile(fixture.configs[node], []byte(config), 0600))
		digest := sha256.Sum256([]byte(config))
		t.Logf("NATIVE_QUORUM_CONFIG node=%d client=127.0.0.1:%d route=127.0.0.1:%d sha256=%x", node, base+node, base+3+node, digest)
	}
	for _, listener := range listeners {
		_ = listener.Close()
	}
	listeners = nil
	for node := range fixture.nodes {
		fixture.start(node)
	}
	t.Setenv("NATS_TEST_PORT_BASE", strconv.Itoa(base))
	return fixture
}

func (f *nativeRuntimeBrokers) start(node int) {
	f.t.Helper()
	require.Nil(f.t, f.nodes[node], "restart only a stopped test-owned node")
	log := &nativeRuntimeBrokerLog{}
	command := exec.Command(f.binary, "-c", f.configs[node])
	command.Env = f.env
	command.Stdout, command.Stderr = log, log
	require.NoError(f.t, command.Start())
	process := &nativeRuntimeBroker{command: command, done: make(chan struct{}), log: log}
	f.nodes[node] = process
	go func() { process.err = command.Wait(); close(process.done) }()
	f.t.Logf("NATIVE_QUORUM_START node=%d owned_pid=%d executable=%s at=%s", node, command.Process.Pid, f.binary, time.Now().UTC().Format(time.RFC3339Nano))
	require.Eventually(f.t, func() bool {
		select {
		case <-process.done:
			return false
		default:
		}
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", f.base+node), 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
		return false
	}, 5*time.Second, 20*time.Millisecond, "test-owned broker starts: %s", log.text())
}

func (f *nativeRuntimeBrokers) stop(node int) {
	f.t.Helper()
	process := f.nodes[node]
	if process == nil {
		return
	}
	select {
	case <-process.done:
	default:
		// This exact handle comes from our own Start, never a PID lookup.
		err := process.command.Process.Kill()
		if err != nil && !errors.Is(err, os.ErrProcessDone) {
			f.t.Errorf("stop test-owned broker node=%d: %v", node, err)
		}
		<-process.done
	}
	f.t.Logf("NATIVE_QUORUM_STOP node=%d owned_pid=%d executable=%s at=%s", node, process.command.Process.Pid, f.binary, time.Now().UTC().Format(time.RFC3339Nano))
	if f.t.Failed() {
		f.t.Logf("NATIVE_QUORUM_LOG node=%d %s", node, process.log.text())
	}
	f.nodes[node] = nil
}
