package coordinationrequest

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProjectIsolatesAuthoritativeRecipientAndKeepsFMPStatus(t *testing.T) {
	route := routeRequest(t, "route", testTime)
	speed, err := New("speed", "EKCH", "flight-1", "EKCH_APP", "1234567", "EKCH_FMH", KindSpeed,
		Payload{Speed: &SpeedPayload{Requested: "220 KT"}}, testTime.Add(time.Second))
	require.NoError(t, err)
	superseded, err := route.Supersede("coordination-request/new-route", testTime.Add(2*time.Second))
	require.NoError(t, err)

	controller, err := Project([]Request{superseded, speed}, Audience{Controller: "EKCH_APP", Role: "EKCH_APP"}, []string{"EKCH_FMH"})
	require.NoError(t, err)
	require.Len(t, controller, 1)
	require.Equal(t, KindSpeed, controller[0].Kind)

	other, err := Project([]Request{superseded, speed}, Audience{Controller: "EKCH_CTR", Role: "EKCH_CTR"}, []string{"EKCH_FMH"})
	require.NoError(t, err)
	require.Empty(t, other)

	fmp, err := Project([]Request{superseded, speed}, Audience{Role: "EKCH_FMH"}, []string{"EKCH_FMH"})
	require.NoError(t, err)
	require.Equal(t, []Kind{KindRouteDirect, KindSpeed}, []Kind{fmp[0].Kind, fmp[1].Kind})
	require.Equal(t, StateSuperseded, fmp[0].State)
}

func TestProjectMakesNoOwnerVisibleOnlyToOriginatingFMP(t *testing.T) {
	request, err := New("unassigned", "EKCH", "flight-1", "", "1234567", "EKCH_FMH", KindRouteDirect,
		Payload{RouteDirect: &RouteDirectPayload{DirectTo: "MONAK"}}, testTime)
	require.NoError(t, err)

	controller, err := Project([]Request{request}, Audience{Controller: "EKCH_APP", Role: "EKCH_APP"}, []string{"EKCH_FMH"})
	require.NoError(t, err)
	require.Empty(t, controller)

	fmp, err := Project([]Request{request}, Audience{Role: "EKCH_FMH"}, []string{"EKCH_FMH"})
	require.NoError(t, err)
	require.Equal(t, RecipientUnassigned, fmp[0].RecipientStatus)
}

func TestProjectIsDeterministicAcrossReplayAndLegacyPayload(t *testing.T) {
	newer := routeRequest(t, "z", testTime.Add(time.Second))
	legacyJSON, err := json.Marshal(routeRequest(t, "a", testTime))
	require.NoError(t, err)
	var legacyMap map[string]any
	require.NoError(t, json.Unmarshal(legacyJSON, &legacyMap))
	delete(legacyMap, "recipient_status")
	legacyJSON, err = json.Marshal(legacyMap)
	require.NoError(t, err)
	var legacy Request
	require.NoError(t, json.Unmarshal(legacyJSON, &legacy))

	first, err := Project([]Request{newer, legacy}, Audience{Role: "EKCH_FMH"}, []string{"EKCH_FMH"})
	require.NoError(t, err)
	second, err := Project([]Request{legacy, newer}, Audience{Role: "EKCH_FMH"}, []string{"EKCH_FMH"})
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, []RequestID{"coordination-request/a", "coordination-request/z"}, []RequestID{first[0].ID, first[1].ID})
	require.Equal(t, RecipientAssigned, first[0].RecipientStatus)
}
