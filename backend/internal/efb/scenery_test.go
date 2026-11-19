package efb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type sceneryStoreStub struct {
	values map[int64]string
}

func (s *sceneryStoreStub) PilotScenery(_ context.Context, cid int64, _ string) (string, error) {
	return s.values[cid], nil
}

func (s *sceneryStoreStub) SetPilotScenery(_ context.Context, cid int64, _ string, value string) error {
	s.values[cid] = value
	return nil
}

func sceneryRequest(t *testing.T, api *WebAPI, method, body string) (int, sceneryPayload) {
	t.Helper()
	req := httptest.NewRequest(method, "/efb/scenery", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	api.handleScenery(rec, req)
	var payload sceneryPayload
	_ = json.Unmarshal(rec.Body.Bytes(), &payload)
	return rec.Code, payload
}

func TestSceneryPreferenceIsStoredPerPilot(t *testing.T) {
	store := &sceneryStoreStub{values: map[int64]string{}}
	api := NewWebAPI(WebAPIConfig{Auth: authStub{}, Sceneries: store})

	if code, payload := sceneryRequest(t, api, http.MethodGet, ""); code != http.StatusOK || payload.Scenery != nil || payload.Airport != "EKCH" {
		t.Fatalf("undeclared scenery: %d %+v", code, payload)
	}
	if code, payload := sceneryRequest(t, api, http.MethodPut, `{"scenery":"FlyTampa"}`); code != http.StatusOK || payload.Scenery == nil || *payload.Scenery != "flytampa" {
		t.Fatalf("declare: %d %+v", code, payload)
	}
	if store.values[1234567] != "flytampa" {
		t.Fatalf("stored %+v", store.values)
	}
	if code, payload := sceneryRequest(t, api, http.MethodGet, ""); code != http.StatusOK || payload.Scenery == nil || *payload.Scenery != "flytampa" {
		t.Fatalf("read back: %d %+v", code, payload)
	}
	for _, body := range []string{`{"scenery":"aerosoft"}`, `{}`, `nope`} {
		if code, _ := sceneryRequest(t, api, http.MethodPut, body); code != http.StatusBadRequest {
			t.Fatalf("%s: status %d, want 400", body, code)
		}
	}
	if store.values[1234567] != "flytampa" {
		t.Fatal("a rejected update must not change the stored scenery")
	}
	if code, _ := sceneryRequest(t, NewWebAPI(WebAPIConfig{Auth: authStub{}}), http.MethodGet, ""); code != http.StatusServiceUnavailable {
		t.Fatalf("missing store: %d", code)
	}
}
