package pdc

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseResponse(t *testing.T) {
	c := NewClient("testlogon")

	t.Run("Nested messages from issue", func(t *testing.T) {
		input := "ok {NOZ938 telex {REQUEST PREDEP CLEARANCE NOZ938 B738 TO ENGM AT EKCH STAND A17 ATIS A}} {NOZ938 telex {REQUEST PREDEP CLEARANCE NOZ938 B738 TO ENGM AT EKCH STAND A17 ATIS A}}"

		expected := []Message{
			{
				From:   "NOZ938",
				To:     "telex",
				Type:   "telex",
				Packet: "REQUEST PREDEP CLEARANCE NOZ938 B738 TO ENGM AT EKCH STAND A17 ATIS A",
				Raw:    "NOZ938 telex {REQUEST PREDEP CLEARANCE NOZ938 B738 TO ENGM AT EKCH STAND A17 ATIS A}",
			},
			{
				From:   "NOZ938",
				To:     "telex",
				Type:   "telex",
				Packet: "REQUEST PREDEP CLEARANCE NOZ938 B738 TO ENGM AT EKCH STAND A17 ATIS A",
				Raw:    "NOZ938 telex {REQUEST PREDEP CLEARANCE NOZ938 B738 TO ENGM AT EKCH STAND A17 ATIS A}",
			},
		}

		got, err := c.parseResponse(input)
		require.NoError(t, err)

		if len(got) != len(expected) {
			t.Fatalf("expected %d messages, got %d", len(expected), len(got))
		}

		for i := range expected {
			if !reflect.DeepEqual(got[i], expected[i]) {
				t.Errorf("at index %d:\nexpected %+v\ngot %+v", i, expected[i], got[i])
			}
		}
	})
}

func TestHoppieResponses(t *testing.T) {
	operations := []string{"poll", "cpdlc", "telex"}
	for _, operation := range operations {
		t.Run(operation, func(t *testing.T) {
			for _, tc := range []struct {
				name      string
				status    int
				body      string
				wantError bool
			}{
				{"ok", 200, "ok", false},
				{"whitespace", 200, " \r\nok\t\n", false},
				{"provider rejection", 200, "error {callsign already in use}", true},
				{"empty", 200, "", true},
				{"html", 200, "<html>Unavailable</html>", true},
				{"invalid success prefix", 200, "okay", true},
				{"invalid payload", 200, "ok garbage", true},
				{"non-200 success body", 503, "ok", true},
				{"http failure", 500, "unavailable", true},
				{"too large", 200, strings.Repeat("x", maxHoppieResponseBytes+1), true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						require.Equal(t, operation, r.URL.Query().Get("type"))
						w.WriteHeader(tc.status)
						_, _ = io.WriteString(w, tc.body)
					}))
					t.Cleanup(server.Close)
					client := NewClient("secret-logon")
					client.baseURL = server.URL
					var err error
					switch operation {
					case "poll":
						_, err = client.Poll(context.Background(), "EKCH")
					case "cpdlc":
						err = client.SendCPDLC(context.Background(), "EKCH", "SAS123", "/data2/7//WU/clearance")
					case "telex":
						err = client.SendTelex(context.Background(), "EKCH", "SAS123", "clearance")
					}
					if tc.wantError {
						require.Error(t, err)
						require.NotContains(t, err.Error(), client.logon)
					} else {
						require.NoError(t, err)
					}
				})
			}
		})
	}
}

func TestPollValidatesMessageEnvelope(t *testing.T) {
	for _, tc := range []struct {
		body         string
		wantError    bool
		wantMessages int
	}{
		{"ok", false, 0},
		{"ok\n{SAS123 telex {REQUEST {NESTED}}} {SAS456 cpdlc {/data2/1/7/N/WILCO}}", false, 2},
		{"ok {SAS123 telex {incomplete}", true, 0},
		{"ok {SAS123}", true, 0},
		{"ok {SAS123 telex {valid} trailing garbage}", true, 0},
		{"ok {SAS123 telex extra {valid}}", true, 0},
		{"ok {SAS123 telex {valid}} trailing garbage", true, 0},
		{"ok {SAS123 telex {valid}} }", true, 0},
	} {
		t.Run(tc.body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(server.Close)
			client := NewClient("testlogon")
			client.baseURL = server.URL
			messages, err := client.Poll(context.Background(), "EKCH")
			if tc.wantError {
				require.Error(t, err)
				require.Empty(t, messages, "do not return partial poll results as a successful poll")
			} else {
				require.NoError(t, err)
				require.Len(t, messages, tc.wantMessages)
			}
		})
	}
}

type hoppieTestTransport func(*http.Request) (*http.Response, error)

func (f hoppieTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type hoppieBrokenBody struct{}

func (hoppieBrokenBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (hoppieBrokenBody) Close() error             { return nil }

func TestHoppieTransportAndReadFailures(t *testing.T) {
	for _, operation := range []string{"poll", "send"} {
		for _, readFailure := range []bool{false, true} {
			t.Run(operation+map[bool]string{true: "/read", false: "/transport"}[readFailure], func(t *testing.T) {
				client := NewClient("secret logon+code")
				client.httpClient.Transport = hoppieTestTransport(func(r *http.Request) (*http.Response, error) {
					if readFailure {
						return &http.Response{StatusCode: 200, Body: hoppieBrokenBody{}, Header: make(http.Header)}, nil
					}
					return nil, &url.Error{Op: "Get", URL: r.URL.String(), Err: context.DeadlineExceeded}
				})
				var err error
				if operation == "poll" {
					_, err = client.Poll(context.Background(), "EKCH")
				} else {
					err = client.SendCPDLC(context.Background(), "EKCH", "SAS123", "/data2/7//WU/clearance")
				}
				require.Error(t, err)
				require.NotContains(t, err.Error(), client.logon)
				require.NotContains(t, err.Error(), url.QueryEscape(client.logon))
				if !readFailure {
					require.True(t, errors.Is(err, context.DeadlineExceeded))
				}
			})
		}
	}
}

func TestHoppieRedactsProviderResponse(t *testing.T) {
	client := NewClient("secret logon+code")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "error {"+client.logon+" "+url.QueryEscape(client.logon)+"}")
	}))
	t.Cleanup(server.Close)
	client.baseURL = server.URL
	err := client.SendCPDLC(context.Background(), "EKCH", "SAS123", "/data2/7//WU/clearance")
	require.Error(t, err)
	require.Contains(t, err.Error(), "[REDACTED]")
	require.NotContains(t, err.Error(), client.logon)
	require.NotContains(t, err.Error(), url.QueryEscape(client.logon))
}
