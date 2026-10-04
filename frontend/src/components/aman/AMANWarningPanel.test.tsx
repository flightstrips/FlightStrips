import {fireEvent, render, screen} from "@testing-library/react";
import {describe, expect, it, vi} from "vitest";

import type {AMANCurrentWarnings} from "@/store/aman-warning-store";
import {AMANWarningPanel} from "./AMANWarningPanel";

const current: AMANCurrentWarnings = {
  snapshot: "available",
  items: [{
    id: "warning:weather", source: "technical_health", component: "weather",
    severity: "warning", code: "stale", message: "Weather input is stale",
  }],
};

describe("AMANWarningPanel", () => {
  it("shows traffic timing diagnostics with navigation to the affected flight", () => {
    const onNavigateToFlight = vi.fn(() => true);
    const warnings: AMANCurrentWarnings = {snapshot: "available", items: [{
      id: 'warning:"traffic_prediction"/"traffic_prediction"/"missing_departure_time"/-/"BAW822"/-',
      source: "traffic_prediction", component: "traffic_prediction", severity: "warning", callsign: "BAW822",
      code: "missing_departure_time", message: "BAW822 is still planned at EGLL: no valid filed off-block time (EOBT). Last EuroScope observation: 12:00 UTC.",
    }]};
    render(<AMANWarningPanel current={warnings} connectionState="connected" presentationStatus="ready" flights={[{callsign: "BAW822"}]} onNavigateToFlight={onNavigateToFlight} />);
    expect(screen.getByText("Source: Traffic prediction")).toBeInTheDocument();
    expect(screen.getByText(/no valid filed off-block time/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", {name: "Primary flight BAW822; select in AMAN"}));
    expect(onNavigateToFlight).toHaveBeenCalledWith("BAW822");
  });
  it("provides a keyboard-focusable named region and non-color warning cues", () => {
    render(<AMANWarningPanel connectionState="connected" current={current} presentationStatus="ready" />);

    const panel = screen.getByRole("region", {name: "Current warnings"});
    expect(panel).toHaveAttribute("tabindex", "0");
    panel.focus();
    expect(panel).toHaveFocus();
    expect(screen.getByRole("list", {name: "Current AMAN warnings"})).toBeInTheDocument();
    expect(screen.getByText("warning")).toBeInTheDocument();
    expect(screen.getByText("Source: Technical health")).toBeInTheDocument();
    expect(screen.getByText("Component: weather")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("1 current warning");
  });

  it("offers primary then related flight navigation with arrow-key movement and screen-reader context", () => {
    const onNavigateToFlight = vi.fn(() => true);
    const warningCurrent: AMANCurrentWarnings = {snapshot: "available", items: [{
      id: "warning:spacing", source: "sequence", severity: "error", code: "spacing_conflict",
      message: "Required spacing is unavailable", callsign: "SAS101", related_callsign: "SAS202",
    }]};
    render(<AMANWarningPanel
      connectionState="connected"
      current={warningCurrent}
      flights={[{callsign: "SAS101"}, {callsign: "SAS202"}]}
      onNavigateToFlight={onNavigateToFlight}
      presentationStatus="ready"
    />);

    const list = screen.getByRole("list", {name: "Current AMAN warnings"});
    const primary = screen.getByRole("button", {name: "Primary flight SAS101; select in AMAN"});
    const related = screen.getByRole("button", {name: "Related flight SAS202; select in AMAN"});
    expect(screen.getByRole("listitem", {name: "error warning from Sequence: Required spacing is unavailable"})).toHaveAttribute("data-severity", "error");
    expect(primary.compareDocumentPosition(related) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

    primary.focus();
    fireEvent.keyDown(list, {key: "ArrowDown"});
    expect(related).toHaveFocus();
    fireEvent.keyDown(list, {key: "Home"});
    expect(primary).toHaveFocus();
    fireEvent.click(related);
    expect(onNavigateToFlight).toHaveBeenCalledWith("SAS202");
    expect(screen.getByText("Related flight SAS202 selected.")).toBeInTheDocument();
  });

  it("keeps absent flights explicit and restores focus when the focused warning resolves", () => {
    const warningCurrent: AMANCurrentWarnings = {snapshot: "available", items: [{
      id: "warning:flight", source: "sequence", severity: "warning", code: "late",
      message: "Flight input is late", callsign: "SAS303",
    }]};
    const {rerender} = render(<AMANWarningPanel
      connectionState="connected"
      current={warningCurrent}
      flights={[]}
      presentationStatus="ready"
    />);
    expect(screen.getByLabelText("Primary flight SAS303 unavailable")).toHaveTextContent("unavailable");

    rerender(<AMANWarningPanel
      connectionState="connected"
      current={warningCurrent}
      flights={[{callsign: "SAS303"}]}
      onNavigateToFlight={() => true}
      presentationStatus="ready"
    />);
    screen.getByRole("button", {name: /Primary flight SAS303/}).focus();
    rerender(<AMANWarningPanel connectionState="connected" current={{snapshot: "available", items: []}} presentationStatus="ready" />);
    expect(screen.getByRole("region", {name: "Current warnings"})).toHaveFocus();
    expect(screen.getByText(/focused warning is no longer current/i)).toBeInTheDocument();
  });

  it.each([
    ["degraded", "connected", "Degraded"],
    ["ready", "disconnected", "Disconnected"],
  ] as const)("announces %s data while %s", (presentationStatus, connectionState, expected) => {
    render(<AMANWarningPanel connectionState={connectionState} current={current} presentationStatus={presentationStatus} />);
    expect(screen.getByRole("status")).toHaveTextContent(expected);
  });

  it("distinguishes an old publisher omission from a current empty snapshot", () => {
    const {rerender} = render(<AMANWarningPanel connectionState="connected" current={{items: [], snapshot: "omitted"}} presentationStatus="ready" />);
    expect(screen.getByRole("status")).toHaveTextContent("unavailable from this AMAN publisher");
    rerender(<AMANWarningPanel connectionState="connected" current={{items: [], snapshot: "available"}} presentationStatus="ready" />);
    expect(screen.getByRole("status")).toHaveTextContent("No current warnings");
  });
});
