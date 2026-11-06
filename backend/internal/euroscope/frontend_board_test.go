package euroscope

import (
	"FlightStrips/internal/airportworkflow"
	"FlightStrips/pkg/events"
	"github.com/stretchr/testify/require"
	"testing"
)

func frontendTestBoard(t *testing.T, member *Client) events.BoardContext {
	t.Helper()
	backendWorkflowFixture()
	workflows, err := airportworkflow.NewRegistry(backendFixtureDefinitions, airportworkflow.EKCHDescriptor())
	require.NoError(t, err)
	member.workflow, _ = workflows.Lookup("EKCH")
	member.definition = member.workflow.Definition()
	scope := member.wireContext()
	return events.BoardContext{SessionID: scope.SessionId, Airport: scope.Airport, Environment: scope.Environment, InstanceID: scope.InstanceId, MembershipID: scope.MembershipId, TransportGeneration: scope.TransportGeneration, Generation: scope.Generation, DefinitionRevision: member.definition.Revision(), SubscriptionID: "tab-one"}
}
func TestFrontendBoardUsesExactAuthorizedIncarnation(t *testing.T) {
	hub := membershipTestHub()
	own := membershipTestRoot(hub, "123", "own")
	other := membershipTestRoot(hub, "456", "other")
	a := membershipTestMember(hub, own, 1, "EKCH", "LIVE", "a")
	b := membershipTestMember(hub, own, 2, "EKCH", "SWEATBOX_test", "b")
	membershipTestMember(hub, other, 1, "EKCH", "LIVE", "other")
	board := frontendTestBoard(t, a)
	_ = frontendTestBoard(t, b)
	a.updateIdentity("118.000", "OWN", false, "127.0.0.2")
	controller, definition, readonly, err := hub.AuthorizeFrontendBoard("123", board)
	require.NoError(t, err)
	require.Equal(t, "OWN", controller.Callsign)
	require.Equal(t, a.workflow.ActionCatalog(), definition.Actions)
	require.False(t, readonly)
	binding, err := hub.CaptureFrontendBoardBinding("123", board)
	require.NoError(t, err)
	require.Same(t, a, binding.(*commandBinding).client, "another master must never replace the own instance")
	for _, change := range []func(*events.BoardContext){
		func(b *events.BoardContext) { b.SessionID = 2 }, func(b *events.BoardContext) { b.Airport = "AAAA" }, func(b *events.BoardContext) { b.Environment = "SWEATBOX_test" }, func(b *events.BoardContext) { b.InstanceID = "other" }, func(b *events.BoardContext) { b.MembershipID = "other" }, func(b *events.BoardContext) { b.TransportGeneration++ }, func(b *events.BoardContext) { b.Generation++ }, func(b *events.BoardContext) { b.DefinitionRevision = "old" }, func(b *events.BoardContext) { b.SubscriptionID = "" },
	} {
		wrong := board
		change(&wrong)
		_, _, _, err := hub.AuthorizeFrontendBoard("123", wrong)
		require.Error(t, err)
		_, err = hub.CaptureFrontendBoardBinding("123", wrong)
		require.Error(t, err)
	}
	_, _, _, err = hub.AuthorizeFrontendBoard("456", board)
	require.Error(t, err)
	_, _, ip := hub.FrontendBoardRuntimeState("123", board)
	require.Equal(t, "127.0.0.2", ip)
	a.updateIdentity("118.000", "OWN", true, "127.0.0.2")
	_, _, readonly, err = hub.AuthorizeFrontendBoard("123", board)
	require.NoError(t, err)
	require.True(t, readonly)
	_, err = hub.CaptureFrontendBoardBinding("123", board)
	require.Error(t, err)
	a.updateIdentity("118.000", "OWN", false, "127.0.0.2")
	a.roleEligible.Store(false)
	_, _, readonly, err = hub.AuthorizeFrontendBoard("123", board)
	require.NoError(t, err)
	require.True(t, readonly)
	_, err = hub.CaptureFrontendBoardBinding("123", board)
	require.Error(t, err)
	a.roleEligible.Store(true)
	hub.RemoveMembership(a, "own plugin lost")
	require.Error(t, binding.Validate())
	_, _, _, err = hub.AuthorizeFrontendBoard("123", board)
	require.Error(t, err)
	replacement := membershipTestMember(hub, own, 1, "EKCH", "LIVE", "a")
	_ = frontendTestBoard(t, replacement)
	_, err = hub.CaptureFrontendBoardBinding("123", board)
	require.Error(t, err, "reuse of a membership ID must not revive an old generation")
}
