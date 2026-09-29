package httpresults

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestCommandIDRequiresOneCanonicalUUID(t *testing.T) {
	id := uuid.NewString()
	for _, tc := range []struct {
		values []string
		valid  bool
	}{
		{[]string{id}, true},
		{nil, false},
		{[]string{strings.ToUpper(id)}, false},
		{[]string{" " + id}, false},
		{[]string{id, id}, false},
		{[]string{"not-a-uuid"}, false},
	} {
		r := httptest.NewRequest(http.MethodPost, "/pdc/request", nil)
		for _, value := range tc.values {
			r.Header.Add("Idempotency-Key", value)
		}
		got, valid := CommandID(r)
		if valid != tc.valid || valid && got != id {
			t.Fatalf("headers %v: %q %t", tc.values, got, valid)
		}
	}
}
