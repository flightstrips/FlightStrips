import {fireEvent, render, screen} from "@testing-library/react";
import {describe, expect, it, vi} from "vitest";
import type {AMANFlight, AMANPendingCommand, AMANState} from "@/api/aman";
import {AMANInsertBehindDialog} from "./AMANInsertBehindDialog";
import {AMANUnslottedFlights} from "./AMANUnslottedFlights";

const target = {callsign: "TARGET", lifecycle_state: "airborne", data_status: "fresh", slot: null, star_family: null} as AMANFlight;
const anchor = {callsign: "ANCHOR", lifecycle_state: "stable", data_status: "fresh", star_family: "TESPI", runway_group_id: "22", slot: {runway_group_id: "22", time: "2026-11-11T12:10:00.000Z", sequence: 1}} as AMANFlight;
const state = {revision: 7, generated_at: "2026-11-11T12:00:00.000Z", active_runway_groups: ["22"], runway_groups: [{id: "22"}], flights: [target, anchor], insertion_star_families: {"22": ["TESPI", "MONAK"]}} as unknown as AMANState;

describe("insert behind", () => {
  it("allows STAR recovery with stale guidance and fresh surveillance", () => {
    const recovering = {...target, data_status: "stale" as const, surveillance_fresh: true, star_selection_required: true};
    const onCommand = vi.fn().mockReturnValue("insert-recovery");
    const {unmount} = render(<AMANUnslottedFlights flights={[recovering]} disabled={false} onInsert={vi.fn()} />);
    expect(screen.getByRole("button", {name: "Insert behind…"})).toBeEnabled();
    unmount();
    render(<AMANInsertBehindDialog flight={recovering} state={state} disabled={false} pendingCommands={{}} commandRejections={{}} onCommand={onCommand} onClose={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Behind aircraft"), {target: {value: "ANCHOR"}});
    fireEvent.change(screen.getByLabelText("STAR entry"), {target: {value: "TESPI"}});
    fireEvent.click(screen.getByRole("button", {name: "Insert behind ANCHOR"}));
    expect(onCommand).toHaveBeenCalledExactlyOnceWith({type: "aman.insert_flight_after", callsign: "TARGET", after_callsign: "ANCHOR", star_family: "TESPI"});
  });

  it("blocks unavailable surveillance even when prediction status is fresh", () => {
    const unavailable = {...target, surveillance_fresh: false};
    const {unmount} = render(<AMANUnslottedFlights flights={[unavailable]} disabled={false} onInsert={vi.fn()} />);
    expect(screen.getByRole("button", {name: "Insert behind…"})).toBeDisabled();
    unmount();
    render(<AMANInsertBehindDialog flight={unavailable} state={state} disabled={false} pendingCommands={{}} commandRejections={{}} onCommand={vi.fn()} onClose={vi.fn()} />);
    expect(screen.getByLabelText("Behind aircraft")).toBeDisabled();
  });

  it("requests a STAR only when unknown and sends one insertion command without an ETA", () => {
    const onCommand = vi.fn().mockReturnValue("insert-1");
    render(<AMANInsertBehindDialog flight={target} state={state} disabled={false} pendingCommands={{}} commandRejections={{}} onCommand={onCommand} onClose={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Behind aircraft"), {target: {value: "ANCHOR"}});
    expect(screen.getByRole("button", {name: "Insert behind ANCHOR"})).toBeDisabled();
    fireEvent.change(screen.getByLabelText("STAR entry"), {target: {value: "TESPI"}});
    fireEvent.click(screen.getByRole("button", {name: "Insert behind ANCHOR"}));
    expect(onCommand).toHaveBeenCalledExactlyOnceWith({type: "aman.insert_flight_after", callsign: "TARGET", after_callsign: "ANCHOR", star_family: "TESPI"});
  });

  it("shows pending, rejection and server-confirmed completion", () => {
    const props = {flight: {...target, star_family: "TESPI"}, state, disabled: false, pendingCommands: {}, commandRejections: {}, onCommand: vi.fn().mockReturnValue("insert-1"), onClose: vi.fn()};
    const {rerender} = render(<AMANInsertBehindDialog {...props} />);
    fireEvent.change(screen.getByLabelText("Behind aircraft"), {target: {value: "ANCHOR"}});
    expect(screen.queryByLabelText("STAR entry")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", {name: "Insert behind ANCHOR"}));
    rerender(<AMANInsertBehindDialog {...props} pendingCommands={{"insert-1": {} as AMANPendingCommand}} />);
    expect(screen.getByRole("button", {name: "Insert behind ANCHOR"})).toBeDisabled();
    expect(screen.getByText("Waiting for the updated sequence…")).toBeInTheDocument();
    rerender(<AMANInsertBehindDialog {...props} commandRejections={{"insert-1": {message: "Aircraft data is stale"} as never}} />);
    expect(screen.getByRole("alert")).toHaveTextContent("Aircraft data is stale");
    rerender(<AMANInsertBehindDialog {...props} state={{...state, revision: 8, flights: [{...target, slot: {...anchor.slot!, sequence: 2}}, anchor]}} />);
    expect(screen.getByText("Server confirmed insertion and updated the sequence.")).toBeInTheDocument();
  });

  it("blocks read-only changes and unavailable aircraft data", () => {
    const props = {state, pendingCommands: {}, commandRejections: {}, onCommand: vi.fn(), onClose: vi.fn()};
    const {rerender} = render(<AMANInsertBehindDialog {...props} flight={target} disabled />);
    expect(screen.getByLabelText("Behind aircraft")).toBeDisabled();
    rerender(<AMANInsertBehindDialog {...props} flight={{...target, data_status: "disconnected"}} disabled={false} />);
    expect(screen.getByLabelText("Behind aircraft")).toBeDisabled();
  });

  it("lists unslotted airborne traffic with its reason, excluding ground and deliberate exclusions", () => {
    const onInsert = vi.fn();
    render(<AMANUnslottedFlights flights={[{...target, prediction_reason: "STAR selection required"}, anchor, {...target, callsign: "GROUND", lifecycle_state: "planned"}, {...target, callsign: "EXCLUDED", sequence_disposition: "desequenced"}]} disabled={false} onInsert={onInsert} />);
    expect(screen.getByText("TARGET")).toBeInTheDocument();
    expect(screen.getByText("STAR selection required")).toBeInTheDocument();
    expect(screen.queryByText("GROUND")).not.toBeInTheDocument();
    expect(screen.queryByText("EXCLUDED")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", {name: "Insert behind…"}));
    expect(onInsert).toHaveBeenCalledWith("TARGET");
  });
});
