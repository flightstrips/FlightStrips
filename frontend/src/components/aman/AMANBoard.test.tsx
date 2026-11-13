import {readFileSync} from "node:fs";
import {resolve} from "node:path";
import {fireEvent, render, screen} from "@testing-library/react";
import {beforeEach, describe, expect, it, vi} from "vitest";

import {replaceAMANState, type AMANState, type AMANStateEvent} from "@/api/aman";
import {AMANBoardView, type AMANBoardViewProps} from "./AMANBoard";

const boardStore = vi.hoisted(() => ({
  amanCommandRejections: {},
  amanFMPAuthority: true,
  amanPendingCommands: {},
  amanSelectedView: "ALL",
  callsign: "",
  controllers: [] as {callsign: string; observer?: boolean}[],
  position: "",
  readOnly: false,
  sendAMANCommand: vi.fn(),
  setAMANSelectedView: vi.fn(),
}));

vi.mock("@/store/store-hooks", () => ({
  useWebSocketStore: (selector: (state: typeof boardStore) => unknown) => selector(boardStore),
}));

const golden = JSON.parse(readFileSync(
  resolve(process.cwd(), "../backend/pkg/events/frontend/testdata/aman-state-v1.json"),
  "utf8",
)) as AMANStateEvent;

function state(): AMANState {
  const value = structuredClone(golden.data);
  value.timeline_configuration = {version: "mapping-v1", mappings: [
    {id: 1, left: "TESPI", right: "TUDLO"},
    {id: 2, left: "MONAK", right: "TIDVU"},
    {id: 3, left: "ERNOV", right: null},
  ]};
  return value;
}

function renderBoard(value: AMANState | null, overrides: Partial<AMANBoardViewProps> = {}) {
  const onSelectFlight = vi.fn();
  render(
    <AMANBoardView
      connectionState="connected"
      error={null}
      onSelectFlight={onSelectFlight}
      presentationStatus={value ? "ready" : "empty"}
      selectedCallsign={null}
      state={value}
      {...overrides}
    />,
  );
  return onSelectFlight;
}

describe("complete AMAN timeline and strips", () => {
  beforeEach(() => {
    localStorage.clear();
    boardStore.sendAMANCommand.mockClear();
    boardStore.callsign = "";
    boardStore.position = "";
    boardStore.controllers = [];
  });

  it("renders null and invalid replacement state explicitly without stale partial data", () => {
    const {rerender} = render(
      <AMANBoardView connectionState="disconnected" error={null} onSelectFlight={() => undefined} presentationStatus="empty" selectedCallsign={null} state={null} />,
    );
    expect(screen.getByText("AMAN timeline unavailable")).toBeInTheDocument();
    expect(screen.getByText(/Waiting for a complete AMAN state replacement/)).toBeInTheDocument();

    rerender(<AMANBoardView connectionState="connected" error="invalid_aman_state" onSelectFlight={() => undefined} presentationStatus="degraded" selectedCallsign={null} state={null} />);
    expect(screen.getByText(/State rejected: invalid_aman_state/)).toBeInTheDocument();
    expect(screen.queryByText("SAS123")).not.toBeInTheDocument();
  });

  it("measures the timeline when its first state arrives after mounting", () => {
    const clientHeight = vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockImplementation(function (this: HTMLElement) {
      return this.querySelector("#aman-timeline-grid") === null ? 0 : 600;
    });
    const {rerender} = render(
      <AMANBoardView connectionState="connected" error={null} onSelectFlight={() => undefined} presentationStatus="empty" selectedCallsign={null} state={null} />,
    );

    rerender(<AMANBoardView connectionState="connected" error={null} onSelectFlight={() => undefined} presentationStatus="ready" selectedCallsign={null} state={state()} />);

    expect(screen.getByTestId("aman-timeline-grid")).toHaveStyle({height: "600px"});
    clientHeight.mockRestore();
  });

  it("renders the timeline when a flight prediction becomes unreliable", () => {
    const data = state();
    data.warnings = [{
      id: 'warning:"traffic_prediction"/"prediction"/"prediction_unreliable"/-/"SAS123"/-',
      source: "traffic_prediction", component: "prediction", severity: "warning", code: "prediction_unreliable",
      callsign: "SAS123", message: "SAS123 arrival estimate remains unstable.",
    }];
    data.flights[0].gain_loss_seconds = null;
    data.flights[0].raw_teta = null;
    data.flights[0].operational_teta = null;
    const result = replaceAMANState(null, {...golden, data});
    expect(result.accepted).toBe(true);
    renderBoard(result.state, {presentationStatus: result.status, error: result.error});
    expect(screen.queryByText("AMAN timeline unavailable")).not.toBeInTheDocument();
    expect(screen.getByRole("button", {name: /Select SAS123/})).toBeInTheDocument();
    expect(screen.getByTestId("operational-marker-SAS123")).toHaveAttribute("data-marker-time", data.flights[0].slot!.time);
  });

  it("renders the normal golden state with backend values and explicit unavailable contract fields", () => {
    renderBoard(state());
    const marker = screen.getByRole("button", {name: /Select SAS123; Stable; current delay G02/});

    expect(screen.getByText("EKCH")).toBeInTheDocument();
    expect(screen.getByRole("button", {name: "22"})).toBeInTheDocument();
    expect(marker).toHaveTextContent("SAS123");
    expect(marker).toHaveTextContent("G02");
    expect(marker).not.toHaveTextContent("Prediction");
    expect(screen.getByTestId("operational-marker-SAS123")).toHaveAttribute("data-marker-time", "2026-07-22T10:18:00.000Z");
  });

  it.each(["FMP", "RWY", "ACC"])("releases the displayed slot after expedite in the %s view and restores it on resume", (view) => {
    const current = state();
    const source = current.flights[0];
    const follower = {
      ...structuredClone(source), callsign: "FOLLOWER", order: 2,
      slot: {...source.slot!, sequence: 2, time: "2026-07-22T10:21:00.000Z"},
    };
    current.flights = [source, follower];
    const props: AMANBoardViewProps = {
      connectionState: "connected", error: null, onSelectFlight: vi.fn(),
      presentationStatus: "ready", selectedCallsign: source.callsign, state: current,
    };
    const {rerender} = render(<AMANBoardView {...props} />);
    if (view !== "FMP") fireEvent.click(screen.getByRole("button", {name: view}));
    expect(screen.getByRole("button", {name: /Select SAS123/})).toBeInTheDocument();

    const confirmed = structuredClone(current);
    confirmed.revision++;
    confirmed.flights[0].sequence_disposition = "desequenced";
    confirmed.flights[0].expedited = true;
    // The backend retains the former slot as history and promotes the follower into it.
    confirmed.flights[1].order = 1;
    confirmed.flights[1].slot = structuredClone(source.slot);
    rerender(<AMANBoardView {...props} state={confirmed} />);
    expect(screen.queryByRole("button", {name: /Select SAS123/})).not.toBeInTheDocument();
    expect(screen.getByRole("button", {name: /Select FOLLOWER/}).closest('[role="listitem"]'))
      .toHaveAttribute("data-marker-time", source.slot!.time);
    expect(confirmed.flights[0].slot).toEqual(source.slot);

    const resumed = structuredClone(confirmed);
    resumed.revision++;
    resumed.flights[0].sequence_disposition = "active";
    resumed.flights[0].expedited = false;
    resumed.flights[0].slot!.time = "2026-07-22T10:21:00.000Z";
    rerender(<AMANBoardView {...props} state={resumed} />);
    expect(screen.getByRole("button", {name: /Select SAS123/}).closest('[role="listitem"]'))
      .toHaveAttribute("data-marker-time", resumed.flights[0].slot!.time);
  });

  it("does not invent operational lane mappings when configuration is unavailable", () => {
    const unconfigured = state();
    delete unconfigured.timeline_configuration;

    renderBoard(unconfigured);

    expect(screen.getByRole("region", {name: "AMAN timeline configuration unavailable"})).toBeInTheDocument();
    expect(screen.getByText(/Waiting for a versioned terminal-layout projection/)).toBeInTheDocument();
    expect(screen.queryByTestId(/^fmp-timeline-/)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", {name: /Select SAS123/})).not.toBeInTheDocument();
  });

  it("keeps frozen operational markers fixed without adding a raw-TETA timeline marker", () => {
    const frozen = state();
    frozen.flights[0].freeze_reason = "superstable";
    frozen.flights[0].raw_teta = "2026-07-22T10:40:00.000Z";
    frozen.flights[0].slot!.time = "2026-07-22T10:18:00.000Z";
    renderBoard(frozen);

    expect(screen.getByTestId("operational-marker-SAS123")).toHaveAttribute("data-marker-time", "2026-07-22T10:18:00.000Z");
    expect(screen.queryByTestId("raw-marker-SAS123")).not.toBeInTheDocument();
    expect(screen.getByRole("button", {name: /Superstable/})).toBeInTheDocument();
  });

  it("golden-renders degraded, stale, go-around, manual freeze, queue, and discrepancy facts", () => {
    const degraded = state();
    const flight = degraded.flights[0];
    degraded.technical_health.status = "degraded";
    degraded.technical_health.ready = false;
    degraded.technical_health.blocked_reasons = ["predictor stale"];
    flight.data_status = "stale";
    flight.lifecycle_state = "go_around";
    flight.freeze_reason = "manual";
    flight.confidence = null;
    flight.provenance = null;
    flight.route_fact = null;
    flight.queue_offers = [{
      callsign: flight.callsign,
      runway_group_id: "ARRIVAL-22",
      candidate_slot: {...flight.slot!, time: "2026-07-22T10:16:00.000Z"},
      queue_position: 1,
      expires_at: "2026-07-22T10:05:00.000Z",
      airport_revision: 7,
      reason: "earlier_available",
    }];
    flight.eta_review = {
      status: "pending",
      created_at: "2026-07-22T10:00:00.000Z",
      deadline_at: "2026-07-22T10:05:00.000Z",
      resolved_at: null,
      actor: null,
      note: null,
      initial_baseline_teta: "2026-07-22T10:22:00.000Z",
      calculated_operational_teta: "2026-07-22T10:19:00.000Z",
      selected_teta: "2026-07-22T10:22:00.000Z",
      manual_teta: null,
    };

    renderBoard(degraded, {presentationStatus: "degraded", connectionState: "disconnected"});
    expect(screen.getAllByText("degraded").length).toBeGreaterThan(0);
    expect(screen.getByText("predictor stale")).toBeInTheDocument();
    expect(screen.getByRole("button", {name: /Select SAS123; go around; current delay G02/})).toBeInTheDocument();
    expect(screen.getByTestId("operational-marker-SAS123")).toHaveTextContent("G02");
  });

  it("does not show guidance from a non-authoritative AMAN state", () => {
    const readOnly = state();
    readOnly.authoritative = false;
    readOnly.effective_mode = "read_only";

    renderBoard(readOnly);

    expect(screen.getByTestId("operational-marker-SAS123")).toHaveTextContent("Unavailable");
    expect(screen.getByTestId("operational-marker-SAS123")).not.toHaveTextContent("G01");
  });

  it("supports compact timeline marker hit testing from the designed scrolling layout", () => {
    const onSelectFlight = vi.fn();
    const onOpenFlightActions = vi.fn();
    renderBoard(state(), {onSelectFlight, onOpenFlightActions});
    const marker = screen.getByRole("button", {name: /Select SAS123/});
    fireEvent.click(marker);

    expect(onSelectFlight).toHaveBeenNthCalledWith(1, "SAS123");
    expect(onOpenFlightActions).toHaveBeenNthCalledWith(1, "SAS123");
    expect(screen.getByTestId("aman-timeline-grid")).toHaveClass("min-w-full");
    expect(screen.getAllByTestId(/^fmp-timeline-/)).toHaveLength(3);
  });

  it("activates a focused compact target through the keyboard click contract", () => {
    const onSelectFlight = vi.fn();
    const onOpenFlightActions = vi.fn();
    renderBoard(state(), {onSelectFlight, onOpenFlightActions});
    const target = screen.getByRole("button", {name: /Select SAS123/});

    target.focus();
    fireEvent.click(target, {detail: 0});

    expect(target).toHaveFocus();
    expect(onSelectFlight).toHaveBeenCalledWith("SAS123");
    expect(onOpenFlightActions).toHaveBeenCalledWith("SAS123");
  });

  it("renders GAP intervals and audited manual exceptions in every timeline view", () => {
    const current = state();
    current.runway_groups[0].gaps = [{id: "gap-1", start: "2026-07-22T10:12:00.000Z", end: "2026-07-22T10:18:00.000Z", label: "approach stop", created_at: "2026-07-22T10:01:00.000Z", created_by: "fmp-1"}];
    current.flights[0].runway_gap_exception = {gap_id: "gap-1", runway_group_id: "ARRIVAL-22", opportunity: "2026-07-22T10:18:00.000Z", command_id: "place-1"};
    renderBoard(current);
    const gap = screen.getAllByRole("button", {name: /GAP ARRIVAL-22: approach stop/})[0];
    fireEvent.click(gap);
    expect(screen.getByRole("dialog", {name: "Remove GAP"})).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", {name: "YES"}));
    expect(boardStore.sendAMANCommand).toHaveBeenCalledWith({type: "aman.remove_gap", runway_group_id: "ARRIVAL-22", gap_id: "gap-1"});
    expect(screen.getByText("GAP EXCEPTION")).toHaveAttribute("title", "Audited manual placement inside GAP gap-1");
    fireEvent.click(screen.getByRole("button", {name: "RWY"}));
    expect(screen.getAllByRole("button", {name: /GAP ARRIVAL-22/}).length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole("button", {name: "ACC"}));
    expect(screen.getAllByRole("button", {name: /GAP ARRIVAL-22/}).length).toBeGreaterThan(0);
  });

  it("renders the selected flight runway GAP in the FMP view", () => {
    const current = state();
    current.runway_groups.unshift({id: "ARRIVAL-04", gaps: []});
    current.runway_groups[1].gaps = [{id: "gap-selected", start: "2026-07-22T10:12:00.000Z", end: "2026-07-22T10:18:00.000Z", label: "selected runway gap", created_at: "2026-07-22T10:01:00.000Z", created_by: "fmp-1"}];

    renderBoard(current, {selectedCallsign: "SAS123"});

    expect(screen.getAllByRole("button", {name: /GAP ARRIVAL-22: selected runway gap/})).toHaveLength(3);
  });

  it("renders a newly focused GAP runway even when the selected flight uses another runway", () => {
    const current = state();
    current.runway_groups.push({
      id: "ARRIVAL-04",
      gaps: [{id: "gap-new", start: "2026-07-22T10:12:00.000Z", end: "2026-07-22T10:18:00.000Z", label: "new gap", created_at: "2026-07-22T10:01:00.000Z", created_by: "fmp-1"}],
    });

    renderBoard(current, {focusedRunwayGroupID: "ARRIVAL-04", selectedCallsign: "SAS123"});

    expect(screen.getAllByRole("button", {name: /GAP ARRIVAL-04: new gap/})).toHaveLength(3);
  });

  it("applies local feeder and runway fields and labels their preferences dialog", () => {
    localStorage.setItem("flightstrips.aman.target-fields.v1", JSON.stringify({version: 1, feeder: ["feeder-fix-eta"], runway: ["runway", "aircraft-type", "wtc"]}));
    const current = state();
    current.flights[0].feeder_fix_eta = "2026-07-22T10:12:00.000Z";
    current.flights[0].aircraft_type = "B38M";
    current.flights[0].wake_category = "M";
    renderBoard(current);

    expect(screen.getByRole("button", {name: /Select SAS123/})).toHaveTextContent("10:12SAS123G02ARRIVAL-22B38MM");
    fireEvent.click(screen.getAllByRole("button", {name: "Open target information preferences"})[0]);
    expect(screen.getByRole("dialog", {name: "Target Information"})).toBeInTheDocument();
  });

  it("omits configured ATYP and WTC cells when the publisher has no value", () => {
    localStorage.setItem("flightstrips.aman.target-fields.v1", JSON.stringify({version: 1, feeder: [], runway: ["aircraft-type", "wtc"]}));
    renderBoard(state());

    const target = screen.getByRole("button", {name: /Select SAS123/});
    expect(target.querySelector('[data-field="aircraft-type"]')).not.toBeInTheDocument();
    expect(target.querySelector('[data-field="wtc"]')).not.toBeInTheDocument();
    expect(target).not.toHaveTextContent("—");
  });

  it("omits the FMP and DETAIL accessory buttons from the toolbar", () => {
    renderBoard(state(), {selectedCallsign: "SAS123", onOpenControls: vi.fn(), onOpenFlightDetails: vi.fn()});

    expect(screen.queryByRole("button", {name: "FMP"})).not.toBeInTheDocument();
    expect(screen.queryByRole("button", {name: "DETAIL"})).not.toBeInTheDocument();
  });

  it("stacks near-simultaneous flight strips in their assigned ruler column", () => {
    const overlapping = state();
    const original = overlapping.flights[0];
    overlapping.flights.push({
      ...structuredClone(original),
      callsign: "SAS124",
      order: 4,
      operational_teta: "2026-07-22T10:18:30.000Z",
      raw_teta: "2026-07-22T10:18:30.000Z",
      slot: {...original.slot!, sequence: 4, time: "2026-07-22T10:18:30.000Z"},
    });

    renderBoard(overlapping);
    expect(screen.getByTestId("operational-marker-SAS123")).toHaveClass("-translate-x-full");
    expect(screen.getByTestId("operational-marker-SAS124")).toHaveClass("-translate-x-full");
  });

  it("keeps the FMP overview complete while runway selection remains local", () => {
    const multiRunwayState = state();
    multiRunwayState.runway_groups.push({id: "ARRIVAL-04"});
    multiRunwayState.flights.push({
      ...structuredClone(multiRunwayState.flights[0]),
      callsign: "SKY404",
      runway_group_id: "ARRIVAL-04",
      holding_fix: "TIDVU",
      order: 2,
    });

    renderBoard(multiRunwayState);
    expect(screen.getByRole("button", {name: /Select SAS123/})).toBeInTheDocument();
    expect(screen.getByRole("button", {name: /Select SKY404/})).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", {name: "22"}));
    fireEvent.click(screen.getByRole("checkbox", {name: /ARRIVAL-04/}));
    fireEvent.click(screen.getByRole("button", {name: "Cancel"}));
    expect(screen.getByRole("button", {name: /Select SKY404/})).toBeInTheDocument();
    expect(screen.getByRole("button", {name: /Select SAS123/})).toBeInTheDocument();
  });

  it("selects the server-declared active runway by default", () => {
    const configured = state();
    configured.runway_groups.unshift({id: "ARRIVAL-04", selected: false});
    configured.active_runway_groups = ["ARRIVAL-22"];

    renderBoard(configured);
    fireEvent.click(screen.getByRole("button", {name: "22"}));

    expect(screen.getByText("22", {selector: "span"})).toHaveClass("text-[#f3d02e]");
    expect(screen.getByText("04")).not.toHaveClass("text-[#f3d02e]");
  });

  it("switches to active runway timelines with the local horizon scale", () => {
    renderBoard(state());

    expect(screen.queryByText("10:00–11:30 UTC · 90 min")).not.toBeInTheDocument();
    expect(screen.getByTestId("aman-timeline-grid")).toHaveStyle({height: "960px"});
    expect(screen.getByTestId("aman-timeline-footer")).toHaveClass("absolute", "bottom-0");
    const scrollRail = screen.getByRole("scrollbar", {name: "Timeline scroll position"});
    expect(scrollRail).toHaveAttribute("tabindex", "0");
    expect(scrollRail).toHaveAttribute("aria-valuenow", "35");
    fireEvent.keyDown(scrollRail, {key: "ArrowDown"});
    expect(scrollRail).toHaveAttribute("aria-valuenow", "35");
    fireEvent.keyDown(scrollRail, {key: "ArrowUp"});
    expect(scrollRail).toHaveAttribute("aria-valuenow", "36");
    fireEvent.keyDown(scrollRail, {key: "Home"});
    expect(scrollRail).toHaveAttribute("aria-valuenow", "90");
    fireEvent.keyDown(scrollRail, {key: "End"});
    expect(scrollRail).toHaveAttribute("aria-valuenow", "35");
    fireEvent.keyDown(scrollRail, {key: "Home"});
    fireEvent.keyDown(scrollRail, {key: "End"});
    expect(scrollRail).toHaveAttribute("aria-valuenow", "35");
    fireEvent.click(screen.getByRole("button", {name: "RWY"}));
    expect(screen.getByTestId("rwy-lane-ARRIVAL-22")).toBeInTheDocument();
    expect(screen.getByTestId("aman-timeline-grid")).toHaveClass("min-w-full");
  });

  it("renders the backend-configured FMP paired timelines without inferring an unused family", () => {
    const configured = state();
    configured.timeline_configuration = {version: "mapping-v1", mappings: [
      {id: 1, left: "TESPI", right: "TUDLO"},
      {id: 2, left: "MONAK", right: "TIDVU"},
      {id: 3, left: "ERNOV", right: null},
    ]};

    renderBoard(configured);

    expect(screen.getAllByTestId(/^fmp-timeline-/)).toHaveLength(3);
    expect(screen.getByRole("region", {name: "Timeline 1: TESPI left, TUDLO right"})).toBeInTheDocument();
    expect(screen.getByLabelText("Unused right side")).toBeInTheDocument();
    expect(screen.getByLabelText(/SAS123 at/)).toHaveAttribute("data-family", "TESPI");
  });

  it("shows only the explicit STAR family on runway markers", () => {
    const identities = state();
    identities.flights[0].star = "LEGACY-STAR";
    identities.flights[0].star_family = "TESPI";
    renderBoard(identities);

    expect(screen.getByTestId("operational-marker-SAS123")).not.toHaveTextContent("TESPI");
    fireEvent.click(screen.getByRole("button", {name: "RWY"}));
    expect(screen.getByTestId("operational-marker-SAS123")).toHaveTextContent("TESPI");
    expect(screen.getByTestId("operational-marker-SAS123")).not.toHaveTextContent("LEGACY-STAR");
  });

  it("dims aircraft outside the locally selected STAR in FMP and RWY views", () => {
    const configured = state();
    configured.timeline_configuration = {version: "mapping-v1", mappings: [{id: 1, left: "TESPI", right: "TUDLO"}]};
    configured.flights[0].star_family = "TESPI";
    configured.flights.push({
      ...structuredClone(configured.flights[0]),
      callsign: "TUDLO2",
      feeder: "TUDLO",
      star: "TUDLO",
      star_family: "TUDLO",
      order: 2,
      slot: {...configured.flights[0].slot!, sequence: 2, time: "2026-07-22T10:20:00.000Z"},
    });

    renderBoard(configured);
    fireEvent.click(screen.getByRole("button", {name: "MAESTRO"}));
    fireEvent.click(screen.getByRole("menuitemcheckbox", {name: "TUDLO"}));
    fireEvent.keyDown(screen.getByRole("menu", {name: "MAESTRO"}), {key: "Escape"});

    expect(screen.getByRole("button", {name: /Select SAS123/})).toHaveAttribute("data-dimmed", "true");
    expect(screen.getByRole("button", {name: /Select SAS123/})).toHaveClass("text-[#747474]");
    expect(screen.getByRole("button", {name: /Select TUDLO2/})).not.toHaveAttribute("data-dimmed");

    fireEvent.click(screen.getByRole("button", {name: "RWY"}));
    expect(screen.getByRole("button", {name: /Select SAS123/})).toHaveAttribute("data-dimmed", "true");
    expect(screen.getByRole("button", {name: /Select TUDLO2/})).not.toHaveAttribute("data-dimmed");

    fireEvent.click(screen.getByRole("button", {name: "MAESTRO"}));
    fireEvent.click(screen.getByRole("menuitemcheckbox", {name: "TESPI"}));
    fireEvent.keyDown(screen.getByRole("menu", {name: "MAESTRO"}), {key: "Escape"});
    expect(screen.getByRole("button", {name: /Select SAS123/})).not.toHaveAttribute("data-dimmed");
    expect(screen.getByRole("button", {name: /Select TUDLO2/})).not.toHaveAttribute("data-dimmed");

    fireEvent.click(screen.getByRole("button", {name: "MAESTRO"}));
    fireEvent.click(screen.getByRole("menuitemcheckbox", {name: "TUDLO"}));
    fireEvent.click(screen.getByRole("menuitemcheckbox", {name: "TESPI"}));
    fireEvent.keyDown(screen.getByRole("menu", {name: "MAESTRO"}), {key: "Escape"});
    expect(screen.getByRole("button", {name: /Select SAS123/})).not.toHaveAttribute("data-dimmed");
  });

  it("selects ACC holding defaults from the controller callsign rather than its frequency", () => {
    boardStore.callsign = "EKDK_B_CTR";
    boardStore.position = "119.555";
    boardStore.controllers = ["EKDK_D_CTR", "EKDK_E_CTR", "EKDK_I_CTR"].map((callsign) => ({callsign}));
    const onViewStateChange = vi.fn();
    renderBoard(state(), {onViewStateChange});

    fireEvent.click(screen.getByRole("button", {name: "ACC"}));
    expect(onViewStateChange).toHaveBeenLastCalledWith({view: "acc", starFamilies: ["MONAK"]});
    fireEvent.click(screen.getByRole("button", {name: "MAESTRO"}));
    expect(screen.getByRole("menuitemcheckbox", {name: "MONAK"})).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("menuitemcheckbox", {name: "TESPI"})).toHaveAttribute("aria-checked", "false");
  });

  it("emphasizes one ACC family without filtering traffic and exposes a local ALL override", () => {
    const configured = state();
    configured.timeline_configuration = {version: "mapping-v1", mappings: [
      {id: 1, left: "TESPI", right: "TUDLO"},
    ]};
    configured.flights.push({
      ...structuredClone(configured.flights[0]),
      callsign: "TUDLO2",
      star_family: "TUDLO",
      order: 2,
      slot: {...configured.flights[0].slot!, sequence: 2, time: "2026-07-22T10:20:00.000Z"},
    });
    const onACCViewChange = vi.fn();
    renderBoard(configured, {accView: "TESPI", onACCViewChange});

    fireEvent.click(screen.getByRole("button", {name: "ACC"}));
    expect(screen.getByRole("button", {name: /Select SAS123.*emphasized STAR family/})).toHaveAttribute("data-emphasis", "primary");
    const subdued = screen.getByRole("button", {name: /Select TUDLO2.*other STAR family/});
    expect(subdued).toHaveAttribute("data-emphasis", "subdued");
    expect(subdued).toHaveClass("text-[#686868]");
    expect(screen.getByRole("button", {name: /Select SAS123.*emphasized STAR family/})).toHaveClass("text-[#96d796]");
    expect(screen.queryByText("★ TESPI")).not.toBeInTheDocument();
    expect(screen.getAllByRole("listitem")).toHaveLength(2);

    fireEvent.change(screen.getByRole("combobox", {name: "ACC STAR family emphasis"}), {target: {value: "ALL"}});
    expect(onACCViewChange).toHaveBeenCalledWith("ALL");
  });

});
