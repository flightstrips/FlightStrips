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
		p.atisDataURL = atisSrv.URL
		p.httpClient = atisSrv.Client()
	}
	return p
}

// --- parseAtisCallsign ---

func TestParseAtisCallsign_General(t *testing.T) {
	icao, kind := parseAtisCallsign("KJFK_ATIS")
	assert.Equal(t, "KJFK", icao)
	assert.Equal(t, "general", kind)
}

func TestParseAtisCallsign_Arrival(t *testing.T) {
	icao, kind := parseAtisCallsign("KJFK_A_ATIS")
	assert.Equal(t, "KJFK", icao)
	assert.Equal(t, "arr", kind)
}

func TestParseAtisCallsign_Departure(t *testing.T) {
	icao, kind := parseAtisCallsign("KJFK_D_ATIS")
	assert.Equal(t, "KJFK", icao)
	assert.Equal(t, "dep", kind)
}

func TestParseAtisCallsign_LowercaseIsNormalised(t *testing.T) {
	icao, kind := parseAtisCallsign("egll_atis")
	assert.Equal(t, "EGLL", icao)
	assert.Equal(t, "general", kind)
}

func TestParseAtisCallsign_NotAtis_ReturnsEmpty(t *testing.T) {
	icao, kind := parseAtisCallsign("KJFK_TWR")
	assert.Equal(t, "", icao)
	assert.Equal(t, "", kind)
}

func TestParseAtisCallsign_NoSuffix_ReturnsEmpty(t *testing.T) {
	icao, kind := parseAtisCallsign("KJFK")
	assert.Equal(t, "", icao)
	assert.Equal(t, "", kind)
}

// --- fetchAllAtisData ---

func TestFetchAllAtisData_SeparateArrDep(t *testing.T) {
	feed := []afvAtisEntry{
		{Callsign: "KJFK_A_ATIS", AtisCode: "A"},
		{Callsign: "KJFK_D_ATIS", AtisCode: "B"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(feed)
	}))
	defer srv.Close()

	p := newTestPoller(nil, srv)
	result, err := p.fetchAllAtisData(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "A", result["KJFK"].arr)
	assert.Equal(t, "B", result["KJFK"].dep)
}

func TestFetchAllAtisData_GeneralFillsBothSlots(t *testing.T) {
	feed := []afvAtisEntry{
		{Callsign: "EGLL_ATIS", AtisCode: "C"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(feed)
	}))
	defer srv.Close()

	p := newTestPoller(nil, srv)
	result, err := p.fetchAllAtisData(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "C", result["EGLL"].arr)
	assert.Equal(t, "C", result["EGLL"].dep)
}

func TestFetchAllAtisData_GeneralDoesNotOverwriteSpecific(t *testing.T) {
	// Specific arr already set; general should not overwrite it
	feed := []afvAtisEntry{
		{Callsign: "EGLL_A_ATIS", AtisCode: "X"},
		{Callsign: "EGLL_ATIS", AtisCode: "Y"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(feed)
	}))
	defer srv.Close()

	p := newTestPoller(nil, srv)
	result, err := p.fetchAllAtisData(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "X", result["EGLL"].arr)
	assert.Equal(t, "Y", result["EGLL"].dep) // general fills empty dep slot
}

func TestFetchAllAtisData_EmptyFeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]afvAtisEntry{})
	}))
	defer srv.Close()

	p := newTestPoller(nil, srv)
	result, err := p.fetchAllAtisData(context.Background())

	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestFetchAllAtisData_InvalidJSON_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	p := newTestPoller(nil, srv)
	_, err := p.fetchAllAtisData(context.Background())

	assert.ErrorContains(t, err, "parse ATIS feed")
}

func TestFetchAllAtisData_NonOKStatus_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	p := newTestPoller(nil, srv)
	_, err := p.fetchAllAtisData(context.Background())

	assert.ErrorContains(t, err, "503")
}

func TestFetchAllAtisData_IgnoresNonAtisCallsigns(t *testing.T) {
	feed := []afvAtisEntry{
		{Callsign: "KJFK_TWR", AtisCode: "A"},
		{Callsign: "KJFK_ATIS", AtisCode: "B"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(feed)
	}))
	defer srv.Close()

	p := newTestPoller(nil, srv)
	result, err := p.fetchAllAtisData(context.Background())

	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "B", result["KJFK"].dep)
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
