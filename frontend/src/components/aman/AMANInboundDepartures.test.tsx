import {fireEvent, render, screen} from "@testing-library/react";
import {describe, expect, it} from "vitest";
import {EventType} from "@/api/models";
import type {InboundDeparturesEvent} from "@/api/inbound-departures";
import {AMANInboundDepartures} from "./AMANInboundDepartures";
import {AMANCdmNonCompliance} from "./AMANCdmNonCompliance";

const snapshot: InboundDeparturesEvent = {
  type: EventType.FrontendInboundDepartures, session_id: 1, airport: "EKCH", enabled: true, stale: false,
  updated_at: "2026-10-13T20:43:00Z",
  flights: [{
    callsign: "FBT08F", departure: "LFLL", arrival: "EKCH", ctot: "1942", atot: "1924", eta: "2120",
    status: "early", deviation_seconds: -1080, outside_window_seconds: 780,
  }],
  ctot_flights: [{
    callsign: "FBT08F", departure: "LFLL", arrival: "EKCH", eobt: "1830", tobt: "1830", ctot: "1942", atot: "1924", eta: "2120",
    cdm_status: "CDM", regulation: "EKCH22", status: "early", deviation_seconds: -1080,
  }, {
    callsign: "SAS1", departure: "EGLL", arrival: "EKCH", eobt: "2000", tobt: "2000", ctot: "2030", atot: "", eta: "2200",
    cdm_status: "", regulation: "", status: "pending", deviation_seconds: 0,
  }],
};

describe("AMAN inbound CTOT list", () => {
  it("lists every inbound CTOT flight including pending ones", () => {
    render(<AMANInboundDepartures snapshot={snapshot} connected />);
    for (const text of ["FBT08F", "SAS1", "19:42Z", "19:24Z", "20:30Z", "EKCH22", "−18m", "Early", "Pending"]) {
      expect(screen.getByText(text)).toBeInTheDocument();
    }
    expect(screen.getByRole("status")).toHaveTextContent("20:43:00Z");
  });

  it("marks data stale on failures or disconnect", () => {
    const {rerender} = render(<AMANInboundDepartures snapshot={{...snapshot, stale: true}} connected />);
    expect(screen.getByRole("status")).toHaveTextContent("Stale data");
    rerender(<AMANInboundDepartures snapshot={snapshot} connected={false} />);
    expect(screen.getByRole("status")).toHaveTextContent("Stale data");
  });

  it("distinguishes awaiting data, an empty list and an unavailable session", () => {
    const {rerender} = render(<AMANInboundDepartures snapshot={{...snapshot, updated_at: "", stale: true, ctot_flights: []}} connected />);
    expect(screen.getByRole("status")).toHaveTextContent("Awaiting vIFF");
    rerender(<AMANInboundDepartures snapshot={{...snapshot, ctot_flights: []}} connected />);
    expect(screen.getByText("No inbound aircraft hold a CTOT.")).toBeInTheDocument();
    rerender(<AMANInboundDepartures snapshot={null} connected />);
    expect(screen.getByText("vIFF arrival monitoring is unavailable for this session.")).toBeInTheDocument();
  });
});

describe("AMAN CDM non-compliance list", () => {
  it("shows early departures and lets a row be closed", () => {
    render(<AMANCdmNonCompliance snapshot={snapshot} />);
    expect(screen.getByText("FBT08F")).toBeInTheDocument();
    fireEvent.click(screen.getByLabelText("Remove FBT08F"));
    expect(screen.queryByText("FBT08F")).not.toBeInTheDocument();
  });
});
