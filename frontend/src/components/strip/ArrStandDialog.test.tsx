import { fireEvent, render, screen, within, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ActionType, EventType } from "@/api/models";
import type { WebSocketState } from "@/store/store";
import { ArrStandDialog } from "./ArrStandDialog";

const state = vi.hoisted(() => ({
  strip: undefined as { communication_type: string } | undefined,
  openTextPilotMenu: vi.fn(),
  sendFrequencyMessage: vi.fn(),
  satEnabled: false,
  amanSessionID: 7,
  strips: [] as WebSocketState["strips"],
  standAssignments: [] as WebSocketState["standAssignments"],
  standBlocks: [] as WebSocketState["standBlocks"],
  standActionRejection: null as WebSocketState["standActionRejection"],
  requestAutomaticStand: vi.fn(),
  requestManualStand: vi.fn(),
  confirmStandOverride: vi.fn(),
  clearStandActionRejection: vi.fn(),
  updateStrip: vi.fn(),
}));

const auth = vi.hoisted(() => ({
  getAccessTokenSilently: vi.fn().mockResolvedValue("test-token"),
  logout: vi.fn(),
}));

vi.mock("@auth0/auth0-react", () => ({ useAuth0: () => auth }));

vi.mock("@/store/store-hooks", () => ({
  useStrip: () => state.strip,
  useIsTwr: () => false,
  useWebSocketStore: (selector: (store: typeof state) => unknown) => selector(state),
}));

function show(currentStand = "A18") {
  const onOpenChange = vi.fn();
  const props = { open: true, callsign: "SAS123", currentStand, onOpenChange };
  return { ...render(<ArrStandDialog {...props} />), props, onOpenChange };
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal("ResizeObserver", class {
    observe() {}
    disconnect() {}
  });
  state.strip = undefined;
  state.satEnabled = false;
  state.strips = [];
  state.standAssignments = [];
  state.standBlocks = [];
  state.standActionRejection = null;
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("arrival stand assignment dialog", () => {
  it("opens the compact session timeline from GRAPHIC and returns without assigning", async () => {
    const now = new Date();
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        generated_at: now.toISOString(),
        sessions: [
          { session_id: 7, name: "LIVE", airport: "EKCH", blocks: [], assignments: [{
            id: 1, callsign: "SAS456", stand: "A18", direction: "ARRIVAL", stage: "ASSIGNED",
            created_at: now.toISOString(), eta: new Date(now.getTime() + 20 * 60_000).toISOString(),
          }] },
          { session_id: 8, name: "SWEATBOX", airport: "EKCH", blocks: [], assignments: [{
            id: 2, callsign: "OTHER123", stand: "A18", direction: "ARRIVAL", stage: "ASSIGNED",
            created_at: now.toISOString(),
          }] },
        ],
      }),
    }));
    const { onOpenChange } = show();
    const graphic = screen.getByRole("button", { name: "GRAPHIC" });
    expect(graphic).toHaveClass("stand-assignment-button-dark");
    expect(graphic.style.top).toBe(screen.getByRole("button", { name: "ESC" }).style.top);
    fireEvent.click(graphic);
    expect(await screen.findByText("SAS456")).toBeInTheDocument();
    expect(screen.queryByText("OTHER123")).not.toBeInTheDocument();
    const sections = screen.getByRole("navigation", { name: "Stand sections" });
    expect(within(sections).getByRole("button", { name: "A" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getAllByText("FREE").length).toBe(25);
    fireEvent.click(within(sections).getByRole("button", { name: "D" }));
    expect(screen.getByText("Section D stand timeline")).toBeInTheDocument();
    for (const stand of ["D1", "D2", "D3", "D4"]) expect(screen.getByText(stand)).toBeInTheDocument();
    expect(screen.getAllByText("FREE")).toHaveLength(4);
    expect(screen.queryByText("SAS456")).not.toBeInTheDocument();
    fireEvent.click(within(sections).getByRole("button", { name: "C" }));
    expect(screen.getByText("Section C stand timeline")).toBeInTheDocument();
    expect(screen.getByText("C26")).toBeInTheDocument();
    expect(screen.queryByText("D1")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "BACK" }));
    expect(screen.queryByText("Stand occupancy timeline")).not.toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Manual stand" })).toHaveValue("A18");
    expect(onOpenChange).not.toHaveBeenCalled();
    expect(state.requestManualStand).not.toHaveBeenCalled();
    expect(state.updateStrip).not.toHaveBeenCalled();
  });

  it("reports timeline fetch errors instead of displaying an empty success", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("Stand API unavailable")));
    show();
    fireEvent.click(screen.getByRole("button", { name: "GRAPHIC" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Stand API unavailable");
  });

  it("shows the assignment mode or stand in smaller parentheses", () => {
    state.satEnabled = true;
    show();
    expect(screen.getByRole("button", { name: "ASSIGN (AUTO)" })).toBeInTheDocument();
    expect(screen.getByText("(AUTO)")).toHaveStyle({ fontSize: "0.75em" });
    fireEvent.click(screen.getByRole("button", { name: "B" }));
    expect(screen.getByRole("button", { name: "ASSIGN" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "B6" }));
    expect(screen.getByRole("button", { name: "ASSIGN (B6)" })).toBeEnabled();
    expect(screen.getByText("(B6)")).toHaveStyle({ fontSize: "0.75em" });
    fireEvent.click(screen.getByRole("button", { name: "AUTO ASSIGN" }));
    expect(screen.getByRole("button", { name: "ASSIGN (AUTO)" })).toBeInTheDocument();
  });

  it("opens with an empty panel when no stand is assigned", () => {
    show("");
    expect(within(screen.getByRole("group", { name: "Stand selection" })).queryAllByRole("button")).toHaveLength(0);
    expect(screen.getByRole("textbox")).toHaveValue("");
    for (const letter of ["A", "B", "C", "D", "E", "F", "G", "H", "HANGAR"]) {
      expect(screen.getByRole("button", { name: letter })).toHaveAttribute("aria-pressed", "false");
    }
    fireEvent.click(screen.getByRole("button", { name: "E" }));
    expect(screen.getByRole("group", { name: "E stands" })).toBeInTheDocument();
  });

  it.each([
    ["A18", "A"], ["B4", "B"], ["C39", "C+D"], ["D2", "C+D"],
    ["E90", "E"], ["F9", "F"], ["G132", "G"], ["H106", "H"],
  ])("opens the assigned area for %s", (stand, panel) => {
    show(stand);
    expect(screen.getByRole("group", { name: `${panel} stands` })).toBeInTheDocument();
    expect(screen.getByRole("textbox")).toHaveValue(stand);
  });

  it("resets an unassigned panel to empty when reopened", () => {
    const { rerender, props } = show("");
    fireEvent.click(screen.getByRole("button", { name: "A" }));
    rerender(<ArrStandDialog {...props} open={false} />);
    rerender(<ArrStandDialog {...props} />);
    expect(within(screen.getByRole("group", { name: "Stand selection" })).queryAllByRole("button")).toHaveLength(0);
  });

  it("draws the A panel and only one H, with HANGAR under WEST", () => {
    show();
    const panel = screen.getByRole("group", { name: "A stands" });
    expect(within(panel).getAllByRole("button")).toHaveLength(26);
    expect(parseFloat(within(panel).getByRole("button", { name: "A34" }).style.top)).toBeCloseTo(8 / 751 * 100);
    expect(screen.getAllByRole("button", { name: "H" })).toHaveLength(1);
    const hangar = screen.getByRole("button", { name: "HANGAR" });
    const west = screen.getByRole("button", { name: "WEST" });
    expect(hangar.style.left).toBe(west.style.left);
    expect(parseFloat(hangar.style.top)).toBeGreaterThan(parseFloat(west.style.top));
  });

  it("shows red badges for active blocks and leaves available stands undecorated", () => {
    state.standBlocks = [{ stand: "A34", block_type: "MANUAL" }];
    state.strips = [{
      callsign: "SAS456",
      stand: "A33",
      bay: "CLEARED",
      tsat: "",
    } as WebSocketState["strips"][number]];
    show();
    expect(within(screen.getByRole("button", { name: "A34" })).getByText("BLOCKED")).toHaveClass("stand-assignment-badge-blocked");
    expect(within(screen.getByRole("button", { name: "A33" })).getByText("SAS456")).toHaveClass("stand-assignment-badge-blocked");
    expect(screen.getByRole("button", { name: "A32" }).querySelector(".stand-assignment-badge")).toBeNull();
    expect(screen.getByRole("button", { name: "A34" }).style.outline).toBe("");
  });

  it("shows confirmed callsigns red and assigned callsigns yellow, excluding the edited flight", () => {
    state.standAssignments = [
      { callsign: "SAS456", stand: "A34", stage: "CONFIRMED", direction: "ARRIVAL", source: "AUTOMATIC" },
      { callsign: "SAS789", stand: "A33", stage: "ASSIGNED", direction: "ARRIVAL", source: "AUTOMATIC" },
      { callsign: "SAS123", stand: "A18", stage: "CONFIRMED", direction: "ARRIVAL", source: "AUTOMATIC", blocks: ["A17"] },
    ];
    show();
    expect(within(screen.getByRole("button", { name: "A34" })).getByText("SAS456")).toHaveClass("stand-assignment-badge-blocked");
    expect(within(screen.getByRole("button", { name: "A33" })).getByText("SAS789")).toHaveClass("stand-assignment-badge-assigned");
    expect(screen.queryByText("SAS123")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "A17" }).querySelector(".stand-assignment-badge")).toBeNull();
  });

  it("gives active blocking priority over assigned yellow and deduplicates callsigns", () => {
    state.standAssignments = [
      { callsign: "SAS456", stand: "A34", stage: "ASSIGNED", direction: "ARRIVAL", source: "AUTOMATIC" },
    ];
    state.standBlocks = [{ stand: "A34", block_type: "OCCUPIED", callsign: "SAS456", blocks: ["A33"] }];
    show();
    expect(within(screen.getByRole("button", { name: "A34" })).getAllByText("SAS456")).toHaveLength(1);
    expect(within(screen.getByRole("button", { name: "A34" })).getByText("SAS456")).toHaveClass("stand-assignment-badge-blocked");
    expect(within(screen.getByRole("button", { name: "A33" })).getByText("SAS456")).toHaveClass("stand-assignment-badge-blocked");
  });

  it("shows parked departure assignments as red occupied callsign badges", () => {
    state.standAssignments = [
      { callsign: "IBK7AZ", stand: "A21", stage: "DEPARTURE_BLOCK", direction: "DEPARTURE", source: "AUTOMATIC" },
    ];
    state.strips = [{
      callsign: "IBK7AZ", stand: "A21", bay: "NOT_CLEARED",
    } as WebSocketState["strips"][number]];
    show();
    const stand = screen.getByRole("button", { name: "A21" });
    expect(within(stand).getAllByText("IBK7AZ")).toHaveLength(1);
    expect(within(stand).getByText("IBK7AZ")).toHaveClass("stand-assignment-badge-blocked");
  });

  it("does not hide physical occupancy when the aircraft also has an assignment", () => {
    state.standAssignments = [
      { callsign: "SAS456", stand: "A34", stage: "ASSIGNED", direction: "ARRIVAL", source: "AUTOMATIC" },
    ];
    state.strips = [{
      callsign: "SAS456", stand: "A33", bay: "STAND",
    } as WebSocketState["strips"][number]];
    show();
    expect(within(screen.getByRole("button", { name: "A33" })).getByText("SAS456")).toHaveClass("stand-assignment-badge-blocked");
    expect(within(screen.getByRole("button", { name: "A34" })).getByText("SAS456")).toHaveClass("stand-assignment-badge-assigned");
  });

  it("keeps an assigned aircraft yellow when a different aircraft blocks the same stand", () => {
    state.standAssignments = [
      { callsign: "SAS456", stand: "A34", stage: "ASSIGNED", direction: "ARRIVAL", source: "AUTOMATIC" },
      { callsign: "SAS789", stand: "A34", stage: "CONFIRMED", direction: "ARRIVAL", source: "AUTOMATIC" },
    ];
    show();
    const stand = screen.getByRole("button", { name: "A34" });
    expect(within(stand).getByText("SAS456")).toHaveClass("stand-assignment-badge-assigned");
    expect(within(stand).getByText("SAS789")).toHaveClass("stand-assignment-badge-blocked");
  });

  it("shrinks a long callsign to fit its badge without changing the stand button position", () => {
    state.standAssignments = [
      { callsign: "LONGCALL123", stand: "A34", stage: "ASSIGNED", direction: "ARRIVAL", source: "AUTOMATIC" },
    ];
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(44);
    const originalStyle = window.getComputedStyle;
    vi.spyOn(window, "getComputedStyle").mockImplementation(element => {
      if (!element.classList.contains("stand-assignment-badge")) return originalStyle(element);
      const style = document.createElement("span").style;
      style.setProperty("font-size", "12px");
      style.setProperty("padding-left", "2px");
      style.setProperty("padding-right", "2px");
      return style;
    });
    const range = document.createRange();
    range.getBoundingClientRect = () => new DOMRect(0, 0, 80, 12);
    vi.spyOn(document, "createRange").mockReturnValue(range);
    show();
    const badge = screen.getByText("LONGCALL123");
    expect(parseFloat(badge.style.fontSize)).toBeCloseTo(12 * 39 / 80);
    expect(badge).toHaveClass("stand-assignment-badge-assigned");
    expect(badge.parentElement).toHaveClass("stand-assignment-badges");
    const stand = screen.getByRole("button", { name: "A34" });
    expect(parseFloat(stand.style.top)).toBeCloseTo(8 / 751 * 100);
  });

  it("applies the FlightPlan raised-control treatment to buttons and the white entry", () => {
    show();
    expect(screen.getByRole("button", { name: "A" })).toHaveClass("stand-assignment-button");
    expect(screen.getByRole("button", { name: "ERASE" })).toHaveClass("stand-assignment-button");
    expect(screen.getByRole("button", { name: "AUTO ASSIGN" })).toHaveClass("stand-assignment-button");
    expect(screen.getByRole("button", { name: "A34" })).toHaveClass("stand-assignment-button");
    expect(screen.getByRole("textbox", { name: "Manual stand" })).toHaveClass("stand-assignment-input");
  });

  it("uses the darker area fill and places W1 between RI and SAS", () => {
    show();
    for (const label of ["RI", "RII", "RIII", "W1", "SAS", "SOUTH", "WEST", "HANGAR"]) {
      expect(screen.getByRole("button", { name: label })).toHaveClass("stand-assignment-button-area");
    }
    const labels = ["RI", "W1", "SAS", "SOUTH", "WEST", "HANGAR"];
    for (let index = 1; index < labels.length; index++) {
      const previous = screen.getByRole("button", { name: labels[index - 1] });
      const current = screen.getByRole("button", { name: labels[index] });
      expect(parseFloat(current.style.top)).toBeGreaterThan(
        parseFloat(previous.style.top) + parseFloat(previous.style.height),
      );
    }
    expect(screen.getByRole("button", { name: "A34" })).toHaveClass("stand-assignment-button-light");
    fireEvent.click(screen.getByRole("button", { name: "W1" }));
    expect(screen.getByRole("textbox")).toHaveValue("W1");
    expect(state.updateStrip).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: /^ASSIGN/ }));
    expect(state.updateStrip).toHaveBeenCalledWith("SAS123", { stand: "W1" });
  });

  it("changes the stand panel for every letter", () => {
    show();
    for (const group of ["B", "C", "D", "E", "F", "G", "H", "A"]) {
      fireEvent.click(screen.getByRole("button", { name: group }));
      const panelName = group === "C" || group === "D" ? "C+D" : group;
      const panel = screen.getByRole("group", { name: `${panelName} stands` });
      expect(within(panel).getAllByRole("button").length).toBeGreaterThan(0);
      for (const button of within(panel).getAllByRole("button")) {
        expect(parseFloat(button.style.left)).toBeGreaterThanOrEqual(0);
        expect(parseFloat(button.style.top)).toBeGreaterThanOrEqual(0);
        expect(parseFloat(button.style.left) + parseFloat(button.style.width)).toBeLessThanOrEqual(100);
        expect(parseFloat(button.style.top) + parseFloat(button.style.height)).toBeLessThanOrEqual(100);
      }
      if (group !== "A") expect(within(panel).queryByRole("button", { name: "A34" })).toBeNull();
    }
  });

  it.each(["RI", "RII", "RIII", "W1", "SAS", "SOUTH", "WEST", "HANGAR"])(
    "highlights only %s when a direct option is selected",
    stand => {
      state.satEnabled = true;
      show();
      fireEvent.click(screen.getByRole("button", { name: "B" }));
      fireEvent.click(screen.getByRole("button", { name: "B6" }));
      fireEvent.click(screen.getByRole("button", { name: stand }));
      const selected = screen.getAllByRole("button", { pressed: true });
      expect(selected).toHaveLength(1);
      expect(selected[0]).toHaveTextContent(stand);
      expect(screen.getByRole("textbox")).toHaveValue(stand);
      expect(within(screen.getByRole("group", { name: "Stand selection" })).queryAllByRole("button")).toHaveLength(0);
      expect(state.requestManualStand).not.toHaveBeenCalled();
      fireEvent.click(screen.getByRole("button", { name: /^ASSIGN/ }));
      expect(state.requestManualStand).toHaveBeenCalledWith("SAS123", stand, 0);
    },
  );

  it("clears a direct option's highlight when a letter is selected", () => {
    state.satEnabled = true;
    show();
    fireEvent.click(screen.getByRole("button", { name: "HANGAR" }));
    fireEvent.click(screen.getByRole("button", { name: "C" }));
    expect(screen.getAllByRole("button", { pressed: true })).toEqual([screen.getByRole("button", { name: "C" })]);
    expect(screen.getByRole("textbox")).toHaveValue("");
  });

  it("opens an assigned HANGAR without showing individual spots", () => {
    show("HANGAR");
    expect(screen.getByRole("textbox")).toHaveValue("HANGAR");
    expect(within(screen.getByRole("group", { name: "Stand selection" })).queryAllByRole("button")).toHaveLength(0);
    expect(screen.getByRole("button", { name: "HANGAR" })).toHaveAttribute("aria-pressed", "true");
  });

  it("stages a selected stand until ASSIGN and preserves direct updates", () => {
    const { onOpenChange } = show();
    fireEvent.click(screen.getByRole("button", { name: "A23" }));
    expect(screen.getByRole("textbox", { name: "Manual stand" })).toHaveValue("A23");
    expect(state.updateStrip).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: /^ASSIGN/ }));
    expect(state.updateStrip).toHaveBeenCalledWith("SAS123", { stand: "A23" });
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("initializes from the current stand and resets after reopening", () => {
    const { rerender, props } = show("B4");
    expect(screen.getByRole("group", { name: "B stands" })).toBeInTheDocument();
    expect(screen.getByRole("textbox")).toHaveValue("B4");
    fireEvent.click(screen.getByRole("button", { name: "B6" }));
    rerender(<ArrStandDialog {...props} open={false} />);
    rerender(<ArrStandDialog {...props} currentStand="C39" />);
    expect(screen.getByRole("textbox")).toHaveValue("C39");
    expect(screen.getByRole("group", { name: "C+D stands" })).toBeInTheDocument();
  });

  it.each([
    ["A", "A", "A34 A33 A32 A31 A30 A28 A27 A26 A25 A23 A22 A21 A20 A19 A18 A4 A7 A6 A9 A8 A11 A50 A17 A15 A14 A12"],
    ["B", "B", "B4 B7 B6 B9 B8 B15 B10 B19 B17"],
    ["C", "C+D", "D1 D2 D3 D4 C26 C27 C28 C29 C30 C33 C32 C35 C34 C37 C36 C39"],
    ["D", "C+D", "D1 D2 D3 D4 C26 C27 C28 C29 C30 C33 C32 C35 C34 C37 C36 C39"],
    ["E", "E", "E20 E25 E22 E27 E24 E29 E36 E31 E33 E35 E90 E89 E88 E87 E86 E85 E84 E83 E82 E71 E74 E70 E72 E73 E75 E76 E77 E78"],
    ["F", "F", "F1 F4 F5 F7 F8 F9 F97 F95 F93 F91 F98 F96 F94 F92 F90 F89"],
    ["G", "G", "G110 G111 G112 G113 G114 G118 G117 G119 G121 G120 G122 G124 G123 G125 G127 G126 G128 G130 G129 G131 G132 G133 G134 G136 G135 G137 G15 G16 G17 G18 G19"],
    ["H", "H", "H106 H105 H104 H103 H102 H101"],
  ])("shows exactly the pictured stands for %s", (letter, panelName, expected) => {
    show();
    fireEvent.click(screen.getByRole("button", { name: letter }));
    const buttons = within(screen.getByRole("group", { name: `${panelName} stands` })).getAllByRole("button");
    expect(buttons.map(button => button.textContent)).toEqual(expected.split(" "));
  });

  it("keeps the combined C+D layout identical for either selector", () => {
    show();
    fireEvent.click(screen.getByRole("button", { name: "C" }));
    const cLayout = within(screen.getByRole("group", { name: "C+D stands" })).getAllByRole("button")
      .map(button => ({ label: button.textContent, style: button.getAttribute("style") }));
    fireEvent.click(screen.getByRole("button", { name: "D" }));
    const dLayout = within(screen.getByRole("group", { name: "C+D stands" })).getAllByRole("button")
      .map(button => ({ label: button.textContent, style: button.getAttribute("style") }));
    expect(dLayout).toEqual(cLayout);
    fireEvent.click(screen.getByRole("button", { name: "C35" }));
    expect(screen.getByRole("textbox")).toHaveValue("C35");
  });

  it.each([
    ["B", "B4", 317, 62],
    ["C", "D1", 400, 15],
    ["E", "E90", 704, 18],
    ["F", "F97", 181, 301],
    ["G", "G132", 530, 551],
    ["H", "H105", 125, 113],
  ])("places the %s panel buttons using the drawing coordinates", (letter, stand, left, top) => {
    show();
    fireEvent.click(screen.getByRole("button", { name: letter }));
    const button = screen.getByRole("button", { name: stand });
    expect(parseFloat(button.style.left)).toBeCloseTo((left - 11) / 774 * 100);
    expect(parseFloat(button.style.top)).toBeCloseTo((top - 7) / 751 * 100);
    expect(parseFloat(button.style.width)).toBeCloseTo(72 / 774 * 100);
    expect(parseFloat(button.style.height)).toBeCloseTo(96 / 751 * 100);
  });

  it("erases the draft without sending and allows clearing a legacy stand", () => {
    show("A18");
    fireEvent.click(screen.getByRole("button", { name: "ERASE" }));
    expect(screen.getByRole("textbox")).toHaveValue("");
    expect(state.updateStrip).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: /^ASSIGN/ }));
    expect(state.updateStrip).toHaveBeenCalledWith("SAS123", { stand: "" });
  });

  it("cancels without sending and disables automatic assignment in legacy sessions", () => {
    const { onOpenChange } = show();
    expect(screen.getByRole("button", { name: "AUTO ASSIGN" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "ESC" }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(state.updateStrip).not.toHaveBeenCalled();
    expect(state.requestAutomaticStand).not.toHaveBeenCalled();
  });

  it("uppercases manual entry and submits it with Enter", () => {
    show();
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "c39" } });
    fireEvent.keyDown(screen.getByRole("textbox"), { key: "Enter" });
    expect(state.updateStrip).toHaveBeenCalledWith("SAS123", { stand: "C39" });
  });

  it("cancels with the Escape key without submitting the draft", () => {
    const { onOpenChange } = show("A18");
    fireEvent.keyDown(screen.getByRole("textbox"), { key: "Escape" });
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(state.updateStrip).not.toHaveBeenCalled();
  });

  it("sends versioned SAT requests and closes only after the response", () => {
    state.satEnabled = true;
    state.standAssignments = [{ callsign: "SAS123", stand: "A18", direction: "ARRIVAL", stage: "RESERVED", source: "AUTOMATIC", version: 4 }];
    const { onOpenChange, rerender, props } = show();
    fireEvent.click(screen.getByRole("button", { name: "A23" }));
    fireEvent.click(screen.getByRole("button", { name: /^ASSIGN/ }));
    expect(state.requestManualStand).toHaveBeenCalledWith("SAS123", "A23", 4);
    expect(state.updateStrip).not.toHaveBeenCalled();
    expect(onOpenChange).not.toHaveBeenCalled();
    state.standAssignments = [{ ...state.standAssignments[0], stand: "A23", version: 5 }];
    rerender(<ArrStandDialog {...props} />);
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("uses the automatic action rather than submitting the current stand", () => {
    state.satEnabled = true;
    show("A18");
    fireEvent.click(screen.getByRole("button", { name: "AUTO ASSIGN" }));
    expect(state.requestAutomaticStand).toHaveBeenCalledWith("SAS123", 0);
    expect(state.requestManualStand).not.toHaveBeenCalled();
  });

  it.each(["", "A18"])("defaults ASSIGN to automatic assignment with current stand '%s'", currentStand => {
    state.satEnabled = true;
    const { onOpenChange } = show(currentStand);
    expect(screen.getByRole("button", { name: "AUTO ASSIGN" })).toHaveAttribute("aria-pressed", "true");
    expect(state.requestAutomaticStand).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: /^ASSIGN/ }));
    expect(state.requestAutomaticStand).toHaveBeenCalledWith("SAS123", 0);
    expect(state.requestManualStand).not.toHaveBeenCalled();
    expect(onOpenChange).not.toHaveBeenCalled();
  });

  it("switches from automatic to manual when a stand is entered", () => {
    state.satEnabled = true;
    show();
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "c39" } });
    expect(screen.getByRole("button", { name: "AUTO ASSIGN" })).toHaveAttribute("aria-pressed", "false");
    fireEvent.click(screen.getByRole("button", { name: /^ASSIGN/ }));
    expect(state.requestManualStand).toHaveBeenCalledWith("SAS123", "C39", 0);
    expect(state.requestAutomaticStand).not.toHaveBeenCalled();
  });

  it("resets to automatic mode on reopening after a manual choice", () => {
    state.satEnabled = true;
    const { rerender, props } = show();
    fireEvent.click(screen.getByRole("button", { name: "A23" }));
    expect(screen.getByRole("button", { name: "AUTO ASSIGN" })).toHaveAttribute("aria-pressed", "false");
    rerender(<ArrStandDialog {...props} open={false} />);
    rerender(<ArrStandDialog {...props} />);
    expect(screen.getByRole("button", { name: "AUTO ASSIGN" })).toHaveAttribute("aria-pressed", "true");
    fireEvent.click(screen.getByRole("button", { name: /^ASSIGN/ }));
    expect(state.requestAutomaticStand).toHaveBeenCalledWith("SAS123", 0);
  });

  it("resets to automatic mode and clears all other highlights after erasing", () => {
    state.satEnabled = true;
    show("A18");
    fireEvent.click(screen.getByRole("button", { name: "B" }));
    fireEvent.click(screen.getByRole("button", { name: "B6" }));
    fireEvent.click(screen.getByRole("button", { name: "ERASE" }));
    expect(screen.getByRole("textbox")).toHaveValue("");
    expect(screen.getAllByRole("button", { pressed: true })).toEqual([screen.getByRole("button", { name: "AUTO ASSIGN" })]);
    expect(within(screen.getByRole("group", { name: "Stand selection" })).queryAllByRole("button")).toHaveLength(0);
    expect(screen.getByRole("button", { name: /^ASSIGN/ })).toBeEnabled();
    expect(state.requestAutomaticStand).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: /^ASSIGN/ }));
    expect(state.requestAutomaticStand).toHaveBeenCalledWith("SAS123", 0);
    expect(state.updateStrip).not.toHaveBeenCalled();
  });

  it.each(["AUTO ASSIGN", "ASSIGN"])("reactivates automatic mode before submitting with %s", submit => {
    state.satEnabled = true;
    show("");
    fireEvent.click(screen.getByRole("button", { name: "B" }));
    expect(screen.getByRole("button", { name: "B" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "AUTO ASSIGN" })).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("button", { name: /^ASSIGN/ })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "B6" }));
    expect(screen.getByRole("textbox")).toHaveValue("B6");
    expect(state.requestManualStand).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "AUTO ASSIGN" }));
    expect(screen.getByRole("button", { name: "AUTO ASSIGN" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "B" })).toHaveAttribute("aria-pressed", "false");
    expect(state.requestAutomaticStand).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: submit === "ASSIGN" ? /^ASSIGN/ : submit }));
    expect(state.requestAutomaticStand).toHaveBeenCalledWith("SAS123", 0);
    expect(state.requestManualStand).not.toHaveBeenCalled();
  });

  it("retains the conflict warning and explicit override confirmation", () => {
    state.satEnabled = true;
    const { rerender, props } = show();
    fireEvent.click(screen.getByRole("button", { name: "A23" }));
    fireEvent.click(screen.getByRole("button", { name: /^ASSIGN/ }));
    state.standActionRejection = {
      type: EventType.FrontendActionRejected,
      callsign: "SAS123",
      action: ActionType.FrontendStandAssignmentManualRequest,
      code: "incompatible_or_occupied",
      reason: "Stand is occupied",
    };
    rerender(<ArrStandDialog {...props} />);
    expect(screen.getByText("Stand is occupied")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "YES" }));
    expect(state.confirmStandOverride).toHaveBeenCalledWith("SAS123", "A23", 0, "Stand is occupied");
  });
});


describe("text-pilot stand requests", () => {
  it("opens taxi-in only after the legacy assignment is accepted", async () => {
    state.strip = { communication_type: "T" };
    state.sendFrequencyMessage.mockResolvedValue({ prepared: true, stand: "A18" });
    show();
    fireEvent.click(screen.getByRole("button", { name: "SEND REQ (A18)" }));
    expect(state.updateStrip).not.toHaveBeenCalled();
    await waitFor(() => expect(state.openTextPilotMenu).toHaveBeenCalledWith({ callsign: "SAS123", variant: "apron-in", stand: "A18" }));
  });
  it("does not open taxi-in when assignment is rejected", async () => {
    state.strip = { communication_type: "T" };
    state.sendFrequencyMessage.mockResolvedValue({ prepared: false, error: "Stand assignment rejected" });
    const { onOpenChange } = show();
    fireEvent.click(screen.getByRole("button", { name: "SEND REQ (A18)" }));
    await waitFor(() => expect(state.sendFrequencyMessage).toHaveBeenCalled());
    expect(state.openTextPilotMenu).not.toHaveBeenCalled(); expect(onOpenChange).not.toHaveBeenCalled();
  });
});
