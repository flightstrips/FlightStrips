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
      message: "Required spacing is unavailable", flight_id: "flight-1", related_flight_id: "flight-2",
    }]};
    render(<AMANWarningPanel
      connectionState="connected"
      current={warningCurrent}
      flights={[{flight_id: "flight-1", callsign: "SAS101"}, {flight_id: "flight-2", callsign: "SAS202"}]}
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
    expect(onNavigateToFlight).toHaveBeenCalledWith("flight-2");
    expect(screen.getByText("Related flight SAS202 selected.")).toBeInTheDocument();
  });

  it("keeps absent flights explicit and restores focus when the focused warning resolves", () => {
    const warningCurrent: AMANCurrentWarnings = {snapshot: "available", items: [{
      id: "warning:flight", source: "sequence", severity: "warning", code: "late",
      message: "Flight input is late", flight_id: "missing-flight",
    }]};
    const {rerender} = render(<AMANWarningPanel
      connectionState="connected"
      current={warningCurrent}
      flights={[]}
      presentationStatus="ready"
    />);
    expect(screen.getByLabelText("Primary flight missing-flight unavailable")).toHaveTextContent("unavailable");

    rerender(<AMANWarningPanel
      connectionState="connected"
      current={warningCurrent}
      flights={[{flight_id: "missing-flight", callsign: "SAS303"}]}
      onNavigateToFlight={() => true}
      presentationStatus="ready"
    />);
    screen.getByRole("button", {name: /Primary flight SAS303/}).focus();
    rerender(<AMANWarningPanel connectionState="connected" current={{snapshot: "available", items: []}} presentationStatus="ready" />);
    expect(screen.getByRole("region", {name: "Current warnings"})).toHaveFocus();
    expect(screen.getByText(/focused warning is no longer current/i)).toBeInTheDocument();
  });

  it.each([
    ["degraded", "connected", "Stale"],
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
