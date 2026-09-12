import {fireEvent, render, screen, waitFor} from "@testing-library/react";
import {useState} from "react";
import {describe, expect, it, vi} from "vitest";

import type {AMANFlightDetail} from "@/api/aman-detail";
import {AMANFlightDetailDialog} from "./AMANFlightDetailDialog";

const {fetchDetail, getToken} = vi.hoisted(() => ({fetchDetail: vi.fn(), getToken: vi.fn().mockResolvedValue("token")}));

vi.mock("@auth0/auth0-react", () => ({
  useAuth0: () => ({getAccessTokenSilently: getToken}),
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

  it("requires confirmation before reporting a missed approach", async () => {
    fetchDetail.mockResolvedValue(detail);
    const onConfirm = vi.fn();
    render(<AMANFlightDetailDialog airport="EKCH" flightID="flight-123" missedApproach={{
      blockReason: null, confirmation: null, confirmed: false, pending: false, onConfirm,
    }} onClose={vi.fn()} />);
    await screen.findByRole("dialog", {name: /SAS123/});
    await screen.findByText("A320");

    fireEvent.click(screen.getByRole("button", {name: "Missed approach"}));
    const confirm = screen.getByRole("button", {name: "Confirm missed approach"});
    expect(confirm).toHaveFocus();
    expect(screen.getByText(/configured ten-minute go-around model/)).toBeInTheDocument();
    expect(onConfirm).not.toHaveBeenCalled();
    fireEvent.click(confirm);
    expect(onConfirm).toHaveBeenCalledOnce();
  });

  it("shows detector evidence and pending, rejected, and server-confirmed states without relying on color", async () => {
    fetchDetail.mockResolvedValue(detail);
    const props = {
      blockReason: null, confirmed: false, pending: true, onConfirm: vi.fn(),
      confirmation: {episode_id: "episode-1", reason: "climb", detected_at: "2026-07-22T12:00:00.000Z", evidence_times: ["2026-07-22T11:59:58.000Z"], status: "pending" as const, decided_at: null, decided_by: null, decision_command_id: null, resulting_revision: null},
      rejection: {code: "stale_revision", message: "state changed"},
    };
    const {rerender} = render(<AMANFlightDetailDialog airport="EKCH" flightID="flight-123" missedApproach={props} onClose={vi.fn()} />);
    await screen.findByRole("dialog", {name: /SAS123/});
    await screen.findByText("A320");
    fireEvent.click(screen.getByRole("button", {name: "Missed approach"}));

    expect(screen.getByText(/detected episode at 12:00:00 from 1 surveillance samples/)).toBeInTheDocument();
    expect(screen.getByRole("status", {name: ""})).toHaveTextContent("Waiting for server confirmation");
    expect(screen.getByRole("alert")).toHaveTextContent("Rejected: state changed (stale_revision)");

    rerender(<AMANFlightDetailDialog airport="EKCH" flightID="flight-123" missedApproach={{...props, pending: false, rejection: null, confirmed: true}} onClose={vi.fn()} />);
    expect(screen.getByText(/Server confirmed missed approach/)).toBeInTheDocument();
    expect(screen.getByRole("button", {name: "Missed approach confirmed"})).toBeDisabled();
  });

  it("exposes the server-backed authorization reason and disables confirmation", async () => {
    fetchDetail.mockResolvedValue(detail);
    render(<AMANFlightDetailDialog airport="EKCH" flightID="flight-123" missedApproach={{
      blockReason: "unauthorized", confirmation: null, confirmed: false, pending: false, onConfirm: vi.fn(),
    }} onClose={vi.fn()} />);
    await screen.findByRole("dialog", {name: /SAS123/});
    await screen.findByText("A320");
    fireEvent.click(screen.getByRole("button", {name: "Missed approach"}));

    expect(screen.getByText("Unavailable: FMP authority is required.")).toBeInTheDocument();
    expect(screen.getByRole("button", {name: "Confirm missed approach"})).toBeDisabled();
  });

  it("requires a focused, explicitly labeled confirmation before removal", async () => {
    fetchDetail.mockResolvedValue(detail);
    const onConfirm = vi.fn();
    render(<AMANFlightDetailDialog airport="EKCH" flightID="flight-123" removal={{
      blockReason: null, confirmed: false, pending: false, onConfirm,
    }} onClose={vi.fn()} />);
    await screen.findByRole("dialog", {name: /SAS123/});
    await screen.findByText("A320");

    fireEvent.click(screen.getByRole("button", {name: "Remove from AMAN"}));
    expect(screen.getByRole("heading", {name: "Confirm AMAN removal"})).toBeInTheDocument();
    const confirm = screen.getByRole("button", {name: "Confirm removal"});
    expect(confirm).toHaveFocus();
    expect(screen.getByText(/authoritative server applies and audits the removal/)).toBeInTheDocument();
    expect(onConfirm).not.toHaveBeenCalled();
    fireEvent.click(confirm);
    expect(onConfirm).toHaveBeenCalledOnce();
  });

  it("shows authorization, pending, rejection, and server confirmation without relying on color", async () => {
    fetchDetail.mockResolvedValue(detail);
    const props = {
      blockReason: "unauthorized" as const, confirmed: false, pending: false, onConfirm: vi.fn(),
      rejection: null,
    };
    const {rerender} = render(<AMANFlightDetailDialog airport="EKCH" flightID="flight-123" removal={props} onClose={vi.fn()} />);
    await screen.findByRole("dialog", {name: /SAS123/});
    await screen.findByText("A320");
    fireEvent.click(screen.getByRole("button", {name: "Remove from AMAN"}));

    expect(screen.getByText("Unavailable: FMP authority is required.")).toBeInTheDocument();
    expect(screen.getByRole("button", {name: "Confirm removal"})).toBeDisabled();

    rerender(<AMANFlightDetailDialog airport="EKCH" flightID="flight-123" removal={{...props, blockReason: null, pending: true, rejection: {code: "stale_revision", message: "state changed"}}} onClose={vi.fn()} />);
    expect(screen.getByText("Waiting for server confirmation")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("Rejected: state changed (stale_revision)");

    rerender(<AMANFlightDetailDialog airport="EKCH" flightID="flight-123" removal={{...props, blockReason: null, confirmed: true}} onClose={vi.fn()} />);
    expect(screen.getByText(/Server confirmed removal from AMAN/)).toBeInTheDocument();
    expect(screen.getByRole("button", {name: "Removed from AMAN"})).toBeDisabled();
  });
});
