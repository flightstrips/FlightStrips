package cluster

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"FlightStrips/internal/cdm"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ViffReadClient interface {
	IFPSByDepartureAirport(context.Context, string) (cdm.BulkIFPSData, error)
	IFPSByCallsignParsed(context.Context, string) (*cdm.IFPSData, error)
	AirportMasters(context.Context) ([]cdm.AirportMaster, error)
}

type ViffReadAdapter struct {
	State  NavigationWeather
	Worker ExternalCallWorker
	Client ViffReadClient
	Now    func() time.Time
}

func (a ViffReadAdapter) now() time.Time {
	if a.Now != nil {
		return a.Now().UTC()
	}
	return time.Now().UTC()
}

func (a ViffReadAdapter) Flights(ctx context.Context, airport string, sessionID int32, deadline time.Time) (bool, error) {
	if a.Client == nil || deadline.IsZero() || sessionID <= 0 || len(airport) != 4 || airport != strings.ToUpper(airport) {
		return false, fmt.Errorf("invalid vIFF flight read")
	}
	if err := viffSessionAirport(ctx, a.Worker.Writer, sessionID, airport); err != nil {
		return false, err
	}
	resource := fmt.Sprintf("session/%d", sessionID)
	return a.fetch(ctx, sessionRef(sessionID), "flights/"+resource, resource, func(ctx context.Context) (*pb.ProviderPage, error) {
		rows, err := a.Client.IFPSByDepartureAirport(ctx, airport)
		if err != nil {
			return nil, err
		}
		page, err := typedViffFlights(airport, rows, a.now())
		if err != nil {
			return nil, err
		}
		return &pb.ProviderPage{Provider: "viff", Resource: resource, Parsed: &pb.ProviderPage_ViffFlights{ViffFlights: page}}, nil
	}, deadline)
}

func (a ViffReadAdapter) Flight(ctx context.Context, airport string, sessionID int32, callsign string, deadline time.Time) (bool, error) {
	if a.Client == nil || deadline.IsZero() || sessionID <= 0 || len(airport) != 4 || airport != strings.ToUpper(airport) || callsign == "" || callsign != strings.ToUpper(callsign) {
		return false, fmt.Errorf("invalid vIFF callsign read")
	}
	if err := viffSessionAirport(ctx, a.Worker.Writer, sessionID, airport); err != nil {
		return false, err
	}
	resource := fmt.Sprintf("session/%d/callsign/%s", sessionID, callsign)
	return a.fetch(ctx, sessionRef(sessionID), "flight/"+resource, resource, func(ctx context.Context) (*pb.ProviderPage, error) {
		row, err := a.Client.IFPSByCallsignParsed(ctx, callsign)
		if err != nil {
			return nil, err
		}
		rows := cdm.BulkIFPSData{}
		if row != nil {
			if row.Callsign != callsign {
				return nil, fmt.Errorf("vIFF callsign identity mismatch")
			}
			rows = append(rows, *row)
		}
		page, err := typedViffFlights(airport, rows, a.now())
		if err != nil {
			return nil, err
		}
		return &pb.ProviderPage{Provider: "viff", Resource: resource, Parsed: &pb.ProviderPage_ViffFlights{ViffFlights: page}}, nil
	}, deadline)
}

func (a ViffReadAdapter) Masters(ctx context.Context, airport string, deadline time.Time) (bool, error) {
	if a.Client == nil || deadline.IsZero() || len(airport) != 4 || airport != strings.ToUpper(airport) {
		return false, fmt.Errorf("invalid vIFF master read")
	}
	return a.fetch(ctx, airportRef(airport), "masters/"+airport, "airport-masters", func(ctx context.Context) (*pb.ProviderPage, error) {
		rows, err := a.Client.AirportMasters(ctx)
		if err != nil {
			return nil, err
		}
		page := &pb.ViffMastersPage{FetchedAt: timestamppb.New(a.now())}
		for _, row := range rows {
			page.Masters = append(page.Masters, &pb.ViffMaster{Airport: strings.ToUpper(row.ICAO), Position: row.Position})
		}
		return &pb.ProviderPage{Provider: "viff", Resource: "airport-masters", Parsed: &pb.ProviderPage_ViffMasters{ViffMasters: page}}, nil
	}, deadline)
}

func (a ViffReadAdapter) fetch(ctx context.Context, ref *pb.AggregateRef, identity, resource string, call func(context.Context) (*pb.ProviderPage, error), deadline time.Time) (bool, error) {
	id, err := ProviderEventCommandID("viff-read", "viff", identity+"/"+deadline.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return false, err
	}
	return a.State.FetchProviderPageFor(ctx, a.Worker, id, ref, "viff", resource,
		func(ctx context.Context, _ *pb.ProviderCheckpoint, _ *pb.ProviderPage) (*pb.ProviderPage, *pb.ProviderCheckpoint, error) {
			page, err := call(ctx)
			if err != nil {
				return nil, nil, err
			}
			encoded, err := (proto.MarshalOptions{Deterministic: true}).Marshal(page)
			if err != nil {
				return nil, nil, err
			}
			sum := sha256.Sum256(encoded)
			return page, &pb.ProviderCheckpoint{Provider: "viff", Resource: resource, Etag: hex.EncodeToString(sum[:])}, nil
		})
}

func (a ViffReadAdapter) ReadFlights(ctx context.Context, sessionID int32, callsign string) (*pb.ViffFlightPage, uint64, error) {
	resource := fmt.Sprintf("session/%d", sessionID)
	if callsign != "" {
		resource += "/callsign/" + callsign
	}
	_, page, revision, err := a.State.CheckpointRevisionFor(ctx, sessionRef(sessionID), "viff", resource)
	if err != nil {
		return nil, 0, err
	}
	if page == nil || page.GetViffFlights() == nil {
		return nil, revision, nil
	}
	return proto.Clone(page.GetViffFlights()).(*pb.ViffFlightPage), revision, nil
}

func (a ViffReadAdapter) ReadMasters(ctx context.Context, airport string) (*pb.ViffMastersPage, uint64, error) {
	_, page, revision, err := a.State.CheckpointRevisionFor(ctx, airportRef(airport), "viff", "airport-masters")
	if err != nil {
		return nil, 0, err
	}
	if page == nil || page.GetViffMasters() == nil {
		return nil, revision, nil
	}
	return proto.Clone(page.GetViffMasters()).(*pb.ViffMastersPage), revision, nil
}

func (a ViffReadAdapter) Resume(ctx context.Context, airport string, sessionID int32) error {
	if sessionID > 0 {
		return a.Worker.Resume(ctx, sessionRef(sessionID))
	}
	return a.Worker.Resume(ctx, airportRef(airport))
}

func typedViffFlights(airport string, rows cdm.BulkIFPSData, fetched time.Time) (*pb.ViffFlightPage, error) {
	page := &pb.ViffFlightPage{Airport: airport, FetchedAt: timestamppb.New(fetched)}
	for _, row := range rows {
		if row.Callsign == "" || row.Departure != airport {
			return nil, fmt.Errorf("vIFF flight identity mismatch")
		}
		taxi, err := cdmInt32(row.Taxi)
		if err != nil {
			return nil, err
		}
		page.Flights = append(page.Flights, &pb.ViffFlight{Callsign: row.Callsign, Cid: row.CID, Departure: row.Departure, Arrival: row.Arrival, Eobt: row.EOBT, Tobt: row.TOBT, RequestedTobt: row.ReqTOBT, TaxiMinutes: taxi, Ctot: row.CTOT, Aobt: row.AOBT, Atot: row.ATOT, Eta: row.ETA, MostPenalizingAirspace: row.MostPenalizingAirspace, CdmStatus: row.CDMStatus, AtfcmStatus: row.ATFCMStatus,
			CdmData: &pb.ViffCdmData{Tobt: row.CDMData.TOBT, Tsat: row.CDMData.TSAT, Ttot: row.CDMData.TTOT, Ctot: row.CDMData.CTOT, Reason: row.CDMData.Reason, RequestedTobt: row.CDMData.ReqTOBT, RequestedTobtType: row.CDMData.ReqTOBTType, RequestedAsrt: row.CDMData.ReqASRT, Id: row.CDMData.ID}})
	}
	if err := validateProviderPage(&pb.ProviderPage{Provider: "viff", Resource: "session/1", Parsed: &pb.ProviderPage_ViffFlights{ViffFlights: page}}); err != nil {
		return nil, err
	}
	return page, nil
}
