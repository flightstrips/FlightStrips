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
func handleLocalStripCreation(ctx context.Context, client *Client, message Message) error {
	var req pkg.LocalStripCreation
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	i := client.messageIdentity(ctx)
	service := client.hub.boardActions
	if service == nil {
		return shared.RejectAction("strip board operations unavailable")
	}
	return service.CreateLocalStrip(ctx, i.session, client.GetCid(), i.airport, req)
}
func handleConnectedFlightSearch(ctx context.Context, client *Client, message Message) error {
	var req fe.ConnectedFlightSearchRequest
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	if req.RequestID == "" || len(req.RequestID) > 128 {
		return shared.RejectAction("lookup request identity required")
	}
	i := client.messageIdentity(ctx)
	service := client.hub.boardActions
	if service == nil {
		return shared.RejectAction("strip board operations unavailable")
	}
	flights, err := service.FindConnectedFlights(ctx, i.session, client.GetCid(), i.airport, req.Definition, req.Query)
	if err != nil {
		return err
	}
	if err = shared.ValidateCommandAuthority(ctx, i.session); err != nil {
		return err
	}
	client.Enqueue(fe.ConnectedFlightSearchResult{RequestID: req.RequestID, Flights: flights})
	return nil
}
func handleConnectedFlightAdmission(ctx context.Context, client *Client, message Message) error {
	var req fe.ConnectedFlightAdmissionRequest
	if err := message.JsonUnmarshal(&req); err != nil {
		return err
	}
	i := client.messageIdentity(ctx)
	service := client.hub.boardActions
	if service == nil {
		return shared.RejectAction("strip board operations unavailable")
	}
	return service.AdmitConnectedFlight(ctx, i.session, client.GetCid(), i.airport, req.Definition, req.Flight)
}

type stripBoardOperations interface {
	AnnotateStrip(context.Context, int32, string, string, string, string, pkg.StripRevision, airportworkflow.StripAnnotation) error
	CreateLocalStrip(context.Context, int32, string, string, pkg.LocalStripCreation) error
	FindConnectedFlights(context.Context, int32, string, string, string, string) ([]pkg.ConnectedFlightCandidate, error)
	AdmitConnectedFlight(context.Context, int32, string, string, string, pkg.ConnectedFlightCandidate) error
}

func bindStripBoardOperations(service shared.StripService) stripBoardOperations {
	operations, _ := service.(stripBoardOperations)
	return operations
}
