import {fireEvent, render, screen} from "@testing-library/react";
import {describe, expect, it, vi} from "vitest";

import type {AMANCoordinationRequest, AMANFlight} from "@/api/aman";
import {AMANCoordinationInbox} from "./AMANCoordinationInbox";

const request: AMANCoordinationRequest = {
  id: "request-1", flight_id: "flight-1", recipient_controller: "EKCH_APP", recipient_status: "assigned",
  kind: "route_direct", state: "pending", payload: {route_direct: {direct_to: "TUDLO"}},
  created_at: "2026-09-12T10:00:00Z", updated_at: "2026-09-12T10:00:00Z",
};
const flights = [{flight_id: "flight-1", callsign: "SAS123"}] as AMANFlight[];

describe("controller coordination inbox", () => {
  it("shows an explicit pending state and accepts agreement without a reason", () => {
    const onDecision = vi.fn();
    render(<AMANCoordinationInbox canDecide deciding={false} flights={flights} onDecision={onDecision} requests={[request]} />);
    expect(screen.getByRole("region", {name: "Coordination inbox"})).toHaveTextContent("1 pending request");
    fireEvent.click(screen.getByRole("button", {name: "Review"}));
    expect(screen.getByRole("dialog", {name: "Review coordination request"})).toHaveTextContent("StatusPending");
    expect(screen.getByText(/does not issue a clearance/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", {name: "Accept agreement"}));
    expect(onDecision).toHaveBeenCalledWith("request-1", "accept");
  });

  it("requires a rejection reason and disables decisions when read-only", () => {
    const onDecision = vi.fn();
    const {rerender} = render(<AMANCoordinationInbox canDecide deciding={false} flights={flights} onDecision={onDecision} requests={[request]} />);
    fireEvent.click(screen.getByRole("button", {name: "Review"}));
    fireEvent.click(screen.getByRole("button", {name: "Reject…"}));
    expect(screen.getByRole("button", {name: "Confirm rejection"})).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Rejection reason"), {target: {value: "Traffic"}});
    fireEvent.click(screen.getByRole("button", {name: "Confirm rejection"}));
    expect(onDecision).toHaveBeenCalledWith("request-1", "reject", "Traffic");

    rerender(<AMANCoordinationInbox canDecide={false} deciding={false} flights={flights} onDecision={onDecision} requests={[request]} />);
    expect(screen.getByRole("status")).toHaveTextContent("read-only");
    expect(screen.getByRole("button", {name: "Confirm rejection"})).toBeDisabled();
  });
});
