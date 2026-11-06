package frontend

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"github.com/jackc/pgx/v5"
)

type membershipSelection struct {
	board                *events.BoardContext
	session              int32
	instance, membership string
}

func (s membershipSelection) explicit() bool {
	return s.session != 0 || s.instance != "" || s.membership != ""
}
func selectionFromAuthentication(event events.AuthenticationEvent) (membershipSelection, error) {
	if event.Board != nil {
		board := *event.Board
		if !board.Complete() {
			return membershipSelection{}, shared.RejectAction("complete board context required")
		}
		return membershipSelection{board: &board, session: board.SessionID, instance: board.InstanceID, membership: board.MembershipID}, nil
	}
	selected := membershipSelection{session: event.SessionID, instance: event.PluginInstanceID, membership: event.MembershipID}
	if selected.explicit() && (selected.session <= 0 || selected.instance == "" || selected.membership == "") {
		return selected, shared.RejectAction("complete plugin membership selection required")
	}
	return selected, nil
}

type frontendBoardSource interface {
	AuthorizeFrontendBoard(string, events.BoardContext) (*models.Controller, frontendEvents.BoardDefinition, bool, error)
	CaptureFrontendBoardBinding(string, events.BoardContext) (shared.CommandBinding, error)
}

func (hub *Hub) controllerForSelection(cid string, selected membershipSelection) (*models.Controller, error) {
	if source, ok := hub.server.GetEuroscopeHub().(frontendBoardSource); ok {
		if selected.board == nil {
			return nil, pgx.ErrNoRows
		}
		controller, _, _, err := source.AuthorizeFrontendBoard(cid, *selected.board)
		return controller, err
	}
	if !selected.explicit() {
		return hub.frontendController(cid)
	}
	provider, ok := hub.server.GetEuroscopeHub().(interface {
		GetMembershipController(string, int32, string, string) *models.Controller
	})
	if !ok {
		return nil, shared.RejectAction("membership selection unavailable")
	}
	controller := provider.GetMembershipController(cid, selected.session, selected.instance, selected.membership)
	if controller == nil {
		return nil, pgx.ErrNoRows
	}
	return controller, nil
}
func (hub *Hub) controllerReadOnly(controller *models.Controller, cid string) bool {
	if controller == nil {
		return false
	}
	if _, ok := hub.server.GetEuroscopeHub().(liveFrontendControllerSource); ok {
		return controller.Observer
	}
	esHub := hub.server.GetEuroscopeHub()
	if esHub == nil {
		return false
	}
	return observerInSession(esHub, controller.Session, cid)
}
