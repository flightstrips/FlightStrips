import {render, screen} from "@testing-library/react";
import {describe, expect, it} from "vitest";
import {EventType} from "@/api/models";
import type {InboundDeparturesEvent} from "@/api/inbound-departures";
import {AMANInboundDepartures} from "./AMANInboundDepartures";

const snapshot: InboundDeparturesEvent = {
  type: EventType.FrontendInboundDepartures, session_id: 1, airport: "EKCH", enabled: true, stale: false,
  updated_at: "2026-10-13T20:43:00Z", flights: [{
    callsign: "FBT08F", departure: "LFLL", arrival: "EKCH", ctot: "1942", atot: "1924", eta: "2120",
    status: "early", deviation_seconds: -1080, outside_window_seconds: 780,
  }, {
    callsign: "SAS1", departure: "EGLL", arrival: "EKCH", ctot: "000200", atot: "235159", eta: "0120",
    status: "early", deviation_seconds: -601, outside_window_seconds: 301,
  }],
};

describe("AMAN inbound CTOT list", () => {
  it("shows aircraft, timing, direction, deviation and the amount outside the window", () => {
    render(<AMANInboundDepartures snapshot={snapshot} connected />);
    expect(screen.getByRole("table")).toBeInTheDocument();
    for (const text of ["FBT08F", "LFLL", "19:42Z", "19:24Z", "−18m", "13m", "−10m 1s", "5m 1s"]) {
      expect(screen.getByText(text)).toBeInTheDocument();
    }
    expect(screen.getAllByText("Early")).toHaveLength(2);
    expect(screen.getByText("Inbound aircraft that departed more than 10 minutes before CTOT. All times UTC.")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("20:43:00Z");
  });

  it("keeps findings and marks them stale on failures or disconnect", () => {
    const {rerender} = render(<AMANInboundDepartures snapshot={{...snapshot, stale: true}} connected />);
    expect(screen.getByRole("status")).toHaveTextContent("Stale data");
    expect(screen.getByText("FBT08F")).toBeInTheDocument();
    rerender(<AMANInboundDepartures snapshot={snapshot} connected={false} />);
    expect(screen.getByRole("status")).toHaveTextContent("Stale data");
  });

  it("distinguishes awaiting data, a successful empty list and an unavailable session", () => {
    const {rerender} = render(<AMANInboundDepartures snapshot={{...snapshot, updated_at: "", stale: true, flights: []}} connected />);
    expect(screen.getByRole("status")).toHaveTextContent("Awaiting vIFF");
    expect(screen.queryByText("No inbound aircraft departed more than 10 minutes before CTOT.")).not.toBeInTheDocument();
    rerender(<AMANInboundDepartures snapshot={{...snapshot, flights: []}} connected />);
    expect(screen.getByText("No inbound aircraft departed more than 10 minutes before CTOT.")).toBeInTheDocument();
    rerender(<AMANInboundDepartures snapshot={{...snapshot, stale: true, flights: []}} connected />);
    expect(screen.getByText("No inbound aircraft departed more than 10 minutes before CTOT in the last successful update.")).toBeInTheDocument();
    rerender(<AMANInboundDepartures snapshot={null} connected />);
    expect(screen.getByText("vIFF arrival monitoring is unavailable for this session.")).toBeInTheDocument();
  });
});
