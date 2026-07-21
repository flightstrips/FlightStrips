import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Bay, type TacticalStrip } from "@/api/models";
import { StartButton } from "./TacticalButtons";
import { TacticalCrossingStrip } from "./TacticalCrossingStrip";
import { TacticalMemaidStrip } from "./TacticalMemaidStrip";
import { TacticalRwyStrip } from "./TacticalRwyStrip";

const actions = {
  createTacticalStrip: vi.fn(),
  deleteTacticalStrip: vi.fn(),
  confirmTacticalStrip: vi.fn(),
  forceAssumeTacticalStrip: vi.fn(),
  markTacticalStrip: vi.fn(),
};

let position = "EKCH_TWR";
const controllers = [
  { callsign: "EKCH_TWR", position: "EKCH_TWR", identifier: "TE", section: "TWR", owned_sectors: ["TE"] },
  { callsign: "EKCH_GND", position: "EKCH_GND", identifier: "GE", section: "GND", owned_sectors: ["GE"] },
];

vi.mock("@/store/store-hooks", () => ({
  useWebSocketStore: (selector: (state: unknown) => unknown) => selector({
    ...actions,
    selectedCallsign: "SAS123",
    controllers,
    position,
  }),
  useMyPosition: () => position,
  useControllers: () => controllers,
  useSelectedCallsign: () => "SAS123",
}));

function tactical(overrides: Partial<TacticalStrip> = {}): TacticalStrip {
  return {
    id: 42,
    session_id: 1,
    type: "START",
    bay: Bay.Taxi,
    label: "22L",
    aircraft: "",
    produced_by: "EKCH_TWR",
    owner: "EKCH_TWR",
    marked: false,
    sequence: 1000,
    confirmed: false,
    confirmed_by: "",
    created_at: "2026-07-20T12:00:00Z",
    ...overrides,
  };
}

beforeEach(() => {
  position = "EKCH_TWR";
  Object.values(actions).forEach((mock) => mock.mockReset());
});

afterEach(cleanup);

describe("tactical strip ownership interactions", () => {
  it("lets the owner mark and close a runway strip without a timer control", () => {
    render(<TacticalRwyStrip strip={tactical()} />);

    fireEvent.click(screen.getByText("START 22L"));
    expect(actions.markTacticalStrip).toHaveBeenCalledWith(42, true);
    expect(screen.getByText("✕")).toBeInTheDocument();
    expect(screen.queryByText("⌛")).not.toBeInTheDocument();
  });

  it("opens force assume for a non-owner and omits SI/close controls", () => {
    position = "EKCH_GND";
    const { container } = render(
      <TacticalCrossingStrip strip={tactical({ type: "CROSSING", label: "CROSSING TRAFFIC" })} />,
    );

    expect(screen.queryByText("✕")).not.toBeInTheDocument();
    expect(container.querySelector(".bg-white")).toBeNull();
    fireEvent.click(screen.getByText("CROSSING TRAFFIC"), { clientX: 100, clientY: 120 });

    expect(screen.getByRole("dialog", { name: "Tactical strip actions" })).toHaveStyle({ width: "167px" });
    expect(screen.getByRole("dialog", { name: "Tactical strip actions" }).style.height).toBe("");
    expect(screen.getByText("TE")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "FORCE ASSUME" }));
    expect(actions.forceAssumeTacticalStrip).toHaveBeenCalledWith(42);
  });

  it("keeps the MEMAID acknowledgement separate from force assume", () => {
    position = "EKCH_GND";
    render(<TacticalMemaidStrip strip={tactical({ type: "MEMAID", label: "STOP CLIMB" })} />);

    fireEvent.click(screen.getByText("⌛"));
    expect(actions.confirmTacticalStrip).toHaveBeenCalledWith(42);
    expect(screen.queryByRole("button", { name: "FORCE ASSUME" })).not.toBeInTheDocument();
  });
});

describe("START creation by bay", () => {
  it("creates immediately without a runway in runway bays", () => {
    render(<StartButton bay={Bay.RwyArr} />);
    fireEvent.click(screen.getByRole("button", { name: "START" }));
    expect(actions.createTacticalStrip).toHaveBeenLastCalledWith("START", Bay.RwyArr, "", "SAS123");
  });

  it("requires runway selection in TAXI", () => {
    render(<StartButton bay={Bay.Taxi} />);
    fireEvent.click(screen.getByRole("button", { name: "START" }));
    fireEvent.click(screen.getByRole("button", { name: "22L" }));

    expect(actions.createTacticalStrip).toHaveBeenCalledWith("START", Bay.Taxi, "22L", "SAS123");
  });

  it("requires runway selection in TAXI_LWR", () => {
    render(<StartButton bay={Bay.TaxiLwr} />);
    fireEvent.click(screen.getByRole("button", { name: "START" }));
    fireEvent.click(screen.getByRole("button", { name: "22L" }));

    expect(actions.createTacticalStrip).toHaveBeenCalledWith("START", Bay.TaxiLwr, "22L", "SAS123");
  });
});
