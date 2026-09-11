import {fireEvent, render, screen} from "@testing-library/react";
import {useState} from "react";
import {describe, expect, it, vi} from "vitest";

import type {AMANCoordinationRequest} from "@/api/aman";
import {AMANCoordinationRequestDialog} from "./AMANCoordinationRequestDialog";

const route: AMANCoordinationRequest = {
  id: "route-1", flight_id: "flight-1", recipient_controller: "EKCH_APP", recipient_status: "assigned",
  kind: "route_direct", state: "pending", payload: {route_direct: {direct_to: "TUDLO"}},
  created_at: "2026-09-12T10:00:00.000Z", updated_at: "2026-09-12T10:00:00.000Z",
};

function renderDialog(overrides: Partial<Parameters<typeof AMANCoordinationRequestDialog>[0]> = {}) {
  const props = {callsign: "SAS123", requests: [] as AMANCoordinationRequest[], canSubmit: true, submitting: false, rejection: null, onSubmit: vi.fn(), onClose: vi.fn(), ...overrides};
  render(<AMANCoordinationRequestDialog {...props} />);
  return props;
}

describe("FMP coordination request dialog", () => {
  it("submits route/direct and speed as distinct request kinds", () => {
    const props = renderDialog();
    fireEvent.change(screen.getByLabelText("Direct to"), {target: {value: "tudlo"}});
    fireEvent.click(screen.getByRole("button", {name: "Send request"}));
    expect(props.onSubmit).toHaveBeenCalledWith({kind: "route_direct", direct_to: "TUDLO"});

    fireEvent.click(screen.getByLabelText("Speed"));
    fireEvent.change(screen.getByLabelText("Requested speed"), {target: {value: "M0.78"}});
    fireEvent.click(screen.getByRole("button", {name: "Send request"}));
    expect(props.onSubmit).toHaveBeenLastCalledWith({kind: "speed", requested: "M0.78"});
  });

  it("names the authoritative recipient and same-kind supersession consequence", () => {
    renderDialog({requests: [route]});
    expect(screen.getAllByText(/Recipient:/)[0]).toHaveTextContent("EKCH_APP");
    expect(screen.getByText(/Submitting replaces/)).toHaveTextContent("route/direct");
    expect(screen.getByRole("button", {name: "Replace request"})).toBeDisabled();
  });

  it("shows explicit unassigned recipient and every terminal replacement state", () => {
    const terminal = (["accepted", "rejected", "superseded", "expired"] as const).map((state, index): AMANCoordinationRequest => ({
      ...route, id: `${state}-${index}`, state, recipient_controller: "", recipient_status: "unassigned",
      created_at: `2026-09-12T10:0${index}:00.000Z`, updated_at: `2026-09-12T10:0${index}:00.000Z`,
    }));
    renderDialog({requests: terminal});
    const history = screen.getByRole("region", {name: "Request history"});
    for (const state of ["accepted", "rejected", "superseded", "expired"]) expect(history).toHaveTextContent(state);
    expect(history).toHaveTextContent("Recipient: Unassigned");
    expect(history).toHaveTextContent("Agreed only — awaiting authoritative clearance.");
  });

  it("announces pending submission, rejection and read-only authority", () => {
    const {rerender} = render(<AMANCoordinationRequestDialog callsign="SAS123" requests={[]} canSubmit submitting onSubmit={vi.fn()} onClose={vi.fn()} />);
    expect(screen.getByRole("button", {name: "Submitting…"})).toBeDisabled();
    rerender(<AMANCoordinationRequestDialog callsign="SAS123" requests={[]} canSubmit={false} submitting={false} rejection="revision changed" onSubmit={vi.fn()} onClose={vi.fn()} />);
    expect(screen.getByRole("alert")).toHaveTextContent("Request rejected by server: revision changed");
    expect(screen.getByRole("status")).toHaveTextContent("read-only");
  });

  it("is labelled, validates input, closes with Escape and traps tab focus", () => {
    const props = renderDialog();
    expect(screen.getByRole("dialog", {name: "Coordinate SAS123"})).toBeInTheDocument();
    expect(screen.getByRole("button", {name: "Send request"})).toBeDisabled();
    fireEvent.keyDown(document, {key: "Escape"});
    expect(props.onClose).toHaveBeenCalled();
  });

  it("restores focus to its opener", () => {
    function Harness() {
      const [open, setOpen] = useState(false);
      return <><button onClick={() => setOpen(true)}>Open coordination</button>{open && <AMANCoordinationRequestDialog callsign="SAS123" requests={[]} canSubmit submitting={false} onSubmit={vi.fn()} onClose={() => setOpen(false)} />}</>;
    }
    render(<Harness />);
    const opener = screen.getByRole("button", {name: "Open coordination"});
    opener.focus(); fireEvent.click(opener); fireEvent.keyDown(document, {key: "Escape"});
    expect(opener).toHaveFocus();
  });
});
