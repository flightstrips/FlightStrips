package cdm

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"

	"github.com/google/uuid"
)

// Existing pending CDM exports keep an opaque origin token in their JSON. Only
// the live process keeps the binding. Missing process state (including restart)
// fails closed; it cannot convert deferred controller work into polling work.
type deferredAuthority struct {
	session, stripID      int32
	callsign, kind, value string
	binding               shared.CommandBinding
}

func (c *ActionService) rememberDeferredAuthority(ctx context.Context, session int32, strip *models.Strip, kind, value, existing string) (string, error) {
	if existing != "" {
		_, err := c.restoreDeferredAuthority(ctx, session, strip, kind, value, existing)
		return existing, err
	}
	binding, human := shared.HumanCommand(ctx)
	if !human {
		return "", nil
	}
	if err := shared.ValidateCommandAuthority(ctx, session); err != nil {
		return "", err
	}
	token := uuid.NewString()
	c.service.deferredAuthorities.Store(token, deferredAuthority{session: session, stripID: strip.ID, callsign: strip.Callsign, kind: kind, value: value, binding: binding})
	return token, nil
}

func (c *ActionService) restoreDeferredAuthority(ctx context.Context, session int32, strip *models.Strip, kind, value, token string) (context.Context, error) {
	if token == "" {
		return ctx, shared.ValidateCommandAuthority(ctx, session)
	}
	stored, ok := c.service.deferredAuthorities.Load(token)
	if !ok {
		return ctx, shared.RejectAction("initiating plugin binding for deferred CDM work is unavailable")
	}
	origin := stored.(deferredAuthority)
	if strip == nil || origin.session != session || origin.stripID != strip.ID || origin.callsign != strip.Callsign || origin.kind != kind || origin.value != value {
		return ctx, shared.RejectAction("deferred CDM authority does not match strip identity")
	}
	if err := origin.binding.Validate(); err != nil {
		return ctx, err
	}
	return shared.WithHumanCommand(ctx, origin.binding)
}

type deferredWorkAcknowledger interface {
	AcknowledgeCdmDeferredWork(context.Context, int32, string, string, string, string) (int64, error)
}

func (s *Service) captureViffAuthority(ctx context.Context, session int32, strip *models.Strip, data *models.CdmData) error {
	if _, human := shared.HumanCommand(ctx); !human || s.client == nil || !s.client.isValid || !s.usesViffSession(session) {
		return nil
	}
	token, err := s.actionService.rememberDeferredAuthority(ctx, session, strip, "CDM", "", "")
	if err != nil {
		return err
	}
	data.ViffAuthority = token
	return nil
}

func (c *MasterViffSync) prepareViffAuthority(ctx context.Context, session int32, strip *models.Strip, data *models.CdmData, state viffPushState) (viffPushState, error) {
	state.sourceSet, state.session = true, session
	state.authorityToken = data.ViffAuthority
	if data.ViffAuthority != "" {
		var err error
		ctx, err = c.service.actionService.restoreDeferredAuthority(shared.WithoutCommandAuthority(ctx), session, strip, "CDM", "", data.ViffAuthority)
		if err != nil {
			return state, err
		}
	}
	state.source, _ = shared.HumanCommand(ctx)
	if err := shared.ValidateCommandAuthority(ctx, session); err != nil {
		return state, err
	}
	return state, nil
}

type viffAuthorityAcknowledger interface {
	AcknowledgeCdmViffAuthority(context.Context, int32, string, string) (int64, error)
}

func (c *MasterViffSync) acknowledgeViffAuthority(ctx context.Context, session int32, callsign, token string) error {
	if token == "" {
		return nil
	}
	ack, ok := c.service.stripRepo.(viffAuthorityAcknowledger)
	if !ok {
		return shared.RejectAction("conditional vIFF authority acknowledgement is unavailable")
	}
	_, err := ack.AcknowledgeCdmViffAuthority(ctx, session, callsign, token)
	if err == nil {
		c.service.deferredAuthorities.Delete(token)
	}
	return err
}
