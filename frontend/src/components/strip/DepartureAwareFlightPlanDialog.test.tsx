import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Bay, type FrontendStrip } from "@/api/models";
import { DepartureAwareFlightPlanDialog } from "./DepartureAwareFlightPlanDialog";

const state = vi.hoisted(() => ({
  strip: undefined as FrontendStrip | undefined,
}));

vi.mock("@/store/store-hooks", () => ({
  useAirport: () => "EKCH",
  useStrip: () => state.strip,
}));

vi.mock("@/components/FlightPlanDialog", () => ({
  default: ({ mode, pdcAction }: { mode: string; pdcAction: string }) => (
    <div data-testid="flight-plan-dialog" data-mode={mode} data-pdc-action={pdcAction} />
  ),
}));

describe("DepartureAwareFlightPlanDialog", () => {
  beforeEach(() => {
    state.strip = {
      callsign: "TEST123",
      origin: "EKCH",
      destination: "EKRK",
      bay: Bay.Stand,
      pdc_state: "NONE",
    } as FrontendStrip;
  });

  it("allows clearance from a planned departure bay when requested", () => {
    render(
      <DepartureAwareFlightPlanDialog
        callsign="TEST123"
        open
        onOpenChange={vi.fn()}
        allowClearanceFromPlannedBay
      />,
    );

    expect(screen.getByTestId("flight-plan-dialog")).toHaveAttribute("data-mode", "clearance");
    expect(screen.getByTestId("flight-plan-dialog")).toHaveAttribute("data-pdc-action", "manual");
  });

  it("keeps already-cleared planned departures in view mode", () => {
    state.strip = { ...state.strip!, bay: Bay.Cleared, pdc_state: "CLEARED" };

    render(
      <DepartureAwareFlightPlanDialog
        callsign="TEST123"
        open
        onOpenChange={vi.fn()}
        allowClearanceFromPlannedBay
      />,
    );

    expect(screen.getByTestId("flight-plan-dialog")).toHaveAttribute("data-mode", "view");
    expect(screen.getByTestId("flight-plan-dialog")).toHaveAttribute("data-pdc-action", "default");
  });

  it("does not allow clearance for planned arrivals", () => {
    state.strip = { ...state.strip!, origin: "EKRK", destination: "EKCH" };

    render(
      <DepartureAwareFlightPlanDialog
        callsign="TEST123"
        open
        onOpenChange={vi.fn()}
        allowClearanceFromPlannedBay
      />,
    );

    expect(screen.getByTestId("flight-plan-dialog")).toHaveAttribute("data-mode", "view");
  });
});
