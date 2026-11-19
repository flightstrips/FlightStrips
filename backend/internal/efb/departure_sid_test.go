package efb

import "testing"

func TestWithPublishedSIDVersion(t *testing.T) {
	text := func(v *string) string {
		if v == nil {
			return ""
		}
		return *v
	}
	ref := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	for _, tc := range []struct{ origin, runway, sid, want string }{
		{"EKCH", "22R", "NEXEN", "NEXEN2C"},
		{"EKCH", "04R", "KEMAX", "KEMAX5A"},
		{"EKCH", "04L", "simeg", "SIMEG2E"},
		{"EKCH", "12", "ODON", "ODDON1B"},
		{"EKCH", "30", "SALLO", "SALLO1D"},
		{"EKCH", "22L", "VEDAR", "VEDAR2F"},
		{"EKCH", "22R", "NEXEN2A", "NEXEN2A"}, // a controller's SID is never rewritten
		{"EKCH", "22R", "TUDLO", "TUDLO"},     // not an EKCH SID
		{"EKCH", "", "NEXEN", "NEXEN"},        // runway unknown
		{"EKCH", "18", "NEXEN", "NEXEN"},      // not a departure runway
		{"EDDF", "22R", "NEXEN", "NEXEN"},
		{"EKCH", "22R", "", ""},
	} {
		got := text(withPublishedSIDVersion(tc.origin, ref(tc.runway), ref(tc.sid)))
		if got != tc.want {
			t.Errorf("%s %s %s: got %q, want %q", tc.origin, tc.runway, tc.sid, got, tc.want)
		}
	}
}
