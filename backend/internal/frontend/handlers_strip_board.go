package frontend

import (
	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/shared"
	fe "FlightStrips/pkg/events/frontend"
	pkg "FlightStrips/pkg/models"
	"context"
)

func handleStripAnnotation(ctx context.Context, client *Client, message Message) error {
	var req fe.StripAnnotationEvent
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	i := client.messageIdentity(ctx)
	service := client.hub.boardActions
	if service == nil {
		return shared.RejectAction("strip board operations unavailable")
	}
	return service.AnnotateStrip(ctx, i.session, req.Callsign, client.GetCid(), i.airport, i.position, req.StripRevision, airportworkflow.StripAnnotation{Field: req.Field, Value: req.Value, Enabled: req.Enabled})
}

type stripBoardOperations interface {
	AnnotateStrip(context.Context, int32, string, string, string, string, pkg.StripRevision, airportworkflow.StripAnnotation) error
}

func bindStripBoardOperations(service shared.StripService) stripBoardOperations {
	operations, _ := service.(stripBoardOperations)
	return operations
}
