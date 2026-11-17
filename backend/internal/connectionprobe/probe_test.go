package connectionprobe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestOriginPreservesHostnameAndVerifiesCertificateForHTTPAndUpgrade(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "example.com", r.Host)
		require.Equal(t, "example.com", r.TLS.ServerName)
		require.Equal(t, "FlightStrips-Connection-Probe/1", r.UserAgent())
		w.Header().Set("CF-Ray", "probe-ray")
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		require.Equal(t, "/frontEndEvents", r.URL.Path)
		conn, err := upgrader.Upgrade(w, r, http.Header{"X-Flightstrips-Connection-Id": {"probe-connection"}})
		require.NoError(t, err)
		if conn != nil {
			defer conn.Close()
			_, _, _ = conn.ReadMessage()
		}
	}))
	defer server.Close()
	probe, err := New("https://example.com", strings.TrimPrefix(server.URL, "https://"), time.Second)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate()) // httptest's certificate includes example.com.
	probe.tlsConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	for _, check := range []string{"http", "websocket"} {
		result := probe.Check(context.Background(), "origin", check)
		require.True(t, result.Success, "%+v", result)
		require.Positive(t, result.Duration)
		if check == "websocket" {
			require.Equal(t, "probe-connection", result.ConnectionID)
		}
	}
	probe.base.Host = "wrong-host.invalid"
	result := probe.Check(context.Background(), "origin", "http")
	require.False(t, result.Success, "an origin probe must never bypass hostname verification")
}

func TestCheckBoundsStalledResponsesAndRejectsRedirects(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			http.Redirect(w, r, "https://other.invalid", http.StatusFound)
			return
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	probe, err := New(server.URL, strings.TrimPrefix(server.URL, "https://"), 50*time.Millisecond)
	require.NoError(t, err)
	probe.tlsConfig = server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	result := probe.Check(context.Background(), "origin", "http")
	require.False(t, result.Success)
	require.Equal(t, http.StatusFound, result.Status)
	result = probe.Check(context.Background(), "origin", "websocket")
	require.False(t, result.Success)
	require.Less(t, result.Duration, time.Second)
}

func TestConfigurationRejectsCredentialsAndAmbiguousOrigins(t *testing.T) {
	for _, address := range []string{"http://api.example", "https://user:secret@api.example", "https://api.example/path", "https://api.example?token=secret"} {
		_, err := New(address, "127.0.0.1:443", time.Second)
		require.Error(t, err)
	}
	_, err := New("https://api.example", "127.0.0.1", time.Second)
	require.Error(t, err)
}
