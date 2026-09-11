import {fireEvent, render, screen, waitFor} from "@testing-library/react";
import {useState} from "react";
import {describe, expect, it, vi} from "vitest";

import type {AMANFlightDetail} from "@/api/aman-detail";
import {AMANFlightDetailDialog} from "./AMANFlightDetailDialog";

const {fetchDetail} = vi.hoisted(() => ({fetchDetail: vi.fn()}));

vi.mock("@auth0/auth0-react", () => ({
  useAuth0: () => ({getAccessTokenSilently: vi.fn().mockResolvedValue("token")}),
}));

vi.mock("@/api/aman-detail", async (importOriginal) => ({
  ...await importOriginal<typeof import("@/api/aman-detail")>(),
  fetchAMANFlightDetail: fetchDetail,
}));

const detail: AMANFlightDetail = {
  airport: "EKCH",
  revision: 17,
  generated_at: "2026-07-22T10:00:00.000Z",
  flight: {
    id: "flight-123",
    callsign: "SAS123",
    lifecycle_state: "stable",
    data_status: "fresh",
    runway_group_id: "ARRIVAL-22",
    feeder: "ROSBI",
    star: "ROSBI1A",
    holding_fix: null,
    aircraft_type: "A320",
    wake_category: "M",
    filed_route: null,
  },
  position: null,
  calculation: null,
  filed_route_geometry: null,
  teta_basis: null,
  slot_basis: null,
  holding_plan: null,
};

function Harness() {
  const [open, setOpen] = useState(false);
  return <><button onClick={() => setOpen(true)} type="button">Open SAS123</button>{open && <AMANFlightDetailDialog airport="EKCH" flightID="flight-123" onClose={() => setOpen(false)} />}</>;
}

describe("AMAN flight detail dialog integration", () => {
  it("labels and focuses the modal, shows current backend detail, and restores focus on close", async () => {
    fetchDetail.mockResolvedValue(detail);
    render(<Harness />);
    const opener = screen.getByRole("button", {name: "Open SAS123"});

    opener.focus();
    fireEvent.click(opener);
    const dialog = await screen.findByRole("dialog", {name: "SAS123 — route & prediction detail"});
    expect(screen.getByRole("button", {name: "Close flight detail"})).toHaveFocus();
    expect(dialog).toHaveTextContent("state revision 17");
    expect(dialog).toHaveTextContent("A320");
    expect(dialog).toHaveTextContent("Wake categoryM");

    fireEvent.keyDown(document, {key: "Escape"});
    await waitFor(() => expect(dialog).not.toBeInTheDocument());
    expect(opener).toHaveFocus();
  });

  it("presents TMA slot protection with a visible non-color label", async () => {
    fetchDetail.mockResolvedValue({
      ...detail,
      teta_basis: {
        raw_teta: "2026-07-22T10:20:00.000Z", raw_reta: "2026-07-22T10:20:00.000Z",
        operational_teta: "2026-07-22T10:19:00.000Z", generated_at: detail.generated_at,
        input_observed_at: detail.generated_at, operational_reason: "tma_freeze", freeze_reason: "tma",
        frozen_at: detail.generated_at, confidence: "high", model_version: "model-v1", config_version: "config-v1",
        prediction_basis: "performance_wind", performance_profile_id: "A320", weather_source: "metar",
        sources: ["surveillance"], degradation_reason: null, raw_samples: [], baseline: null, eta_review: null,
      },
    });
    render(<Harness />);
    fireEvent.click(screen.getByRole("button", {name: "Open SAS123"}));

    const dialog = await screen.findByRole("dialog");
    expect(dialog).toHaveTextContent("Freeze protectionTMA entry protection");
    expect(screen.getByText("TMA entry protection")).toHaveClass("bg-cyan-950", "text-cyan-200");
  });
});
