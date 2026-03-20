package metar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestPoller returns a Poller whose base URLs point at the given test servers.
// Pass nil for a server you don't need in a particular test.
func newTestPoller(metarSrv, atisSrv *httptest.Server) *Poller {
	p := &Poller{
		httpClient: &http.Client{},
	}
	if metarSrv != nil {
		p.metarBaseURL = metarSrv.URL
		p.httpClient = metarSrv.Client()
	}
	if atisSrv != nil {
		p.atisBaseURL = atisSrv.URL
		p.httpClient = atisSrv.Client()
	}
	return p
}

// --- fetchAtisCode ---

func TestFetchAtisCode_ReturnsCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/EGKK", r.URL.Path)
		json.NewEncoder(w).Encode([]vatsimAtisStation{{AtisCode: "B"}})
	}))
	defer srv.Close()

	p := newTestPoller(nil, srv)
	code, err := p.fetchAtisCode(context.Background(), "EGKK")

	require.NoError(t, err)
	assert.Equal(t, "B", code)
}

func TestFetchAtisCode_EmptyArray_ReturnsEmptyString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]vatsimAtisStation{})
	}))
	defer srv.Close()

	p := newTestPoller(nil, srv)
	code, err := p.fetchAtisCode(context.Background(), "EGKK")

	require.NoError(t, err)
	assert.Equal(t, "", code)
}

func TestFetchAtisCode_MultipleStations_ReturnsFirst(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]vatsimAtisStation{{AtisCode: "D"}, {AtisCode: "E"}})
	}))
	defer srv.Close()

	p := newTestPoller(nil, srv)
	code, err := p.fetchAtisCode(context.Background(), "EGKK")

	require.NoError(t, err)
	assert.Equal(t, "D", code)
}

func TestFetchAtisCode_InvalidJSON_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	p := newTestPoller(nil, srv)
	_, err := p.fetchAtisCode(context.Background(), "EGKK")

	assert.ErrorContains(t, err, "parse ATIS response")
}

func TestFetchAtisCode_UsesCaseSensitiveICAO(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode([]vatsimAtisStation{{AtisCode: "A"}})
	}))
	defer srv.Close()

	p := newTestPoller(nil, srv)
	_, err := p.fetchAtisCode(context.Background(), "KJFK")

	require.NoError(t, err)
	assert.Equal(t, "/KJFK", gotPath)
}

// --- fetch (METAR) ---

func TestFetch_ReturnsTrimmedMetar(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/EGKK", r.URL.Path)
		w.Write([]byte("  EGKK 201220Z 24010KT 9999 FEW025 15/08 Q1015  \n"))
	}))
	defer srv.Close()

	p := newTestPoller(srv, nil)
	metar, err := p.fetch(context.Background(), "EGKK")

	require.NoError(t, err)
	assert.Equal(t, "EGKK 201220Z 24010KT 9999 FEW025 15/08 Q1015", metar)
}

func TestFetch_EmptyResponse_ReturnsEmptyString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(""))
	}))
	defer srv.Close()

	p := newTestPoller(srv, nil)
	metar, err := p.fetch(context.Background(), "EGKK")

	require.NoError(t, err)
	assert.Equal(t, "", metar)
}
