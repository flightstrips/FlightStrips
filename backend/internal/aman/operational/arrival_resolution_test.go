package operational

import (
	"testing"

	"FlightStrips/internal/aman/navdata"
	"github.com/stretchr/testify/require"
)

func TestFiledTerminalEntry(t *testing.T) {
	tespi, rosbi, tno, after := navdata.FixID("TESPI"), navdata.FixID("ROSBI"), navdata.FixID("TNO"), navdata.FixID("AFTER")
	path := navdata.TerminalPath{Legs: []navdata.ProcedureLeg{
		{FromFix: &tespi, ToFix: &rosbi},
		{FromFix: &rosbi, ToFix: &tno},
		{FromFix: &tno, ToFix: &after},
	}}
	for _, tc := range []struct {
		route string
		want  navdata.FixID
	}{
		{"AAL M725 ADSEN DCT TNO", tno},
		{"aal m725 adsen dct tno/n0250f100", tno},
		{"DCT TESPI DCT TNO TESPI1A", tno},
		{"DCT ROSBI", rosbi},
		{"DCT TESPI TESPI1A", tespi},
		{"DCT KOR", tespi}, // A fix from another arrival cannot replace this entry.
	} {
		t.Run(tc.route, func(t *testing.T) {
			entry := filedTerminalEntry(tc.route, path)
			require.NotNil(t, entry)
			require.Equal(t, tc.want, *entry)
		})
	}
	require.Nil(t, filedTerminalEntry("DCT TNO", navdata.TerminalPath{}))
}
