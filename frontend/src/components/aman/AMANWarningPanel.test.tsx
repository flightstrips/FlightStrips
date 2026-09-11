import {render, screen} from "@testing-library/react";
import {describe, expect, it} from "vitest";

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
