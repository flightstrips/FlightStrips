import {fireEvent, render, screen} from "@testing-library/react";
import {describe, expect, it, vi} from "vitest";
import type {AMANCommandRejection, AMANFlight, AMANPendingCommand} from "@/api/aman";
import {AMANExcludedFlights} from "./AMANExcludedFlights";

const flights = [
  {callsign: "REMOVED", lifecycle_state: "removed", lifecycle_reason: "manual_removal"},
  {callsign: "DESEQUENCED", lifecycle_state: "stable", sequence_disposition: "desequenced"},
  {callsign: "DIVERTED", lifecycle_state: "removed", lifecycle_reason: "diverted"},
  {callsign: "LANDED", lifecycle_state: "landed", sequence_disposition: "desequenced"},
] as AMANFlight[];

describe("excluded aircraft", () => {
  it("restores manual exclusions through the existing resume command", () => {
    const onCommand = vi.fn().mockReturnValue("restore-1");
    render(<AMANExcludedFlights flights={flights} disabled={false} pendingCommands={{}} commandRejections={{}} onCommand={onCommand} />);
    expect(screen.getByText("REMOVED")).toBeInTheDocument();
    expect(screen.getByText("DESEQUENCED")).toBeInTheDocument();
    expect(screen.queryByText("DIVERTED")).not.toBeInTheDocument();
    expect(screen.queryByText("LANDED")).not.toBeInTheDocument();
    fireEvent.click(screen.getAllByRole("button", {name: "Add back to sequence"})[0]);
    expect(onCommand).toHaveBeenCalledWith({type: "aman.resume_flight", callsign: "REMOVED"});
  });

  it("blocks read-only actions and displays pending state and rejection", () => {
    const onCommand = vi.fn().mockReturnValue("restore-1");
    const props = {flights: flights.slice(0, 1), pendingCommands: {}, commandRejections: {}, onCommand};
    const {rerender} = render(<AMANExcludedFlights {...props} disabled />);
    expect(screen.getByRole("button")).toBeDisabled();
    rerender(<AMANExcludedFlights {...props} disabled={false} />);
    fireEvent.click(screen.getByRole("button"));
    rerender(<AMANExcludedFlights {...props} disabled={false} pendingCommands={{"restore-1": {} as AMANPendingCommand}} />);
    expect(screen.getByRole("button", {name: "Adding…"})).toBeDisabled();
    rerender(<AMANExcludedFlights {...props} disabled={false} commandRejections={{"restore-1": {message: "No current arrival prediction"} as AMANCommandRejection}} />);
    expect(screen.getByRole("alert")).toHaveTextContent("No current arrival prediction");
    expect(screen.getByRole("button", {name: "Add back to sequence"})).toBeEnabled();
  });
});
