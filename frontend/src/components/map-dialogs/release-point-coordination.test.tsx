import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ActionType, CommunicationType } from "@/api/models";
import { ApronTaxiMapDialog } from "./ApronTaxiMapDialog";
import { HoldingPointDialog } from "./HoldingPointDialog";
import { TaxiMapDialog } from "./TaxiMapDialog";

const state = vi.hoisted(() => ({
  strip: {
    owner: "",
    communication_type: "T",
    aircraft_type: "A320",
    aircraft_category: "M",
    origin: "EKCH",
    destination: "EGLL",
    release_point: "D",
    unexpected_change_fields: ["release_point"],
  },
  openTextPilotMenu: vi.fn(),
  setReleasePoint: vi.fn(),
  acknowledgeUnexpectedChange: vi.fn(),
  move: vi.fn(),
}));

const position = "EKCH_TWR";

vi.mock("@/store/store-hooks", () => ({
  controllerOwnsTowerSectors: () => false,
  useWebSocketStore: (selector: (value: typeof state & Record<string, unknown>) => unknown) => selector({
    ...state,
    airport: "EKCH",
    controllers: [],
    layout: "",
  }),
  useRunwaySetup: () => ({ departure: ["12"], arrival: [] }),
  useApronOnline: () => false,
  useTwrOnline: () => false,
  useIsTwr: () => true,
  useMyPosition: () => position,
  useStrip: () => state.strip,
}));

const props = { open: true, callsign: "SAS123", onOpenChange: vi.fn() };

beforeEach(() => {
  vi.clearAllMocks();
  state.strip.communication_type = CommunicationType.Text;
  state.strip.owner = position;
});

describe.each([
  {
    name: "tower taxi map",
    dialog: <TaxiMapDialog {...props} moveOnSelect />,
    point: "F",
    draft: { variant: "tower-out" },
  },
  {
    name: "apron taxi map",
    dialog: <ApronTaxiMapDialog {...props} coordinationMode />,
    point: "F",
    draft: { variant: "apron-out" },
  },
  {
    name: "holding point map",
    dialog: <HoldingPointDialog {...props} runway="12" coordinationMode />,
    point: "K2",
    draft: { variant: "tower-out", holdingPoint: true },
  },
])("$name release-point coordination", ({ dialog, point, draft }) => {
  it("updates another controller's release point without opening the text pilot menu", () => {
    state.strip.owner = "EKCH_GND";
    render(dialog);

    fireEvent.click(screen.getByRole("button", { name: point }));

    expect(state.openTextPilotMenu).not.toHaveBeenCalled();
    expect(state.setReleasePoint).toHaveBeenCalledExactlyOnceWith("SAS123", point);
    expect(state.move).not.toHaveBeenCalled();
    expect(state.acknowledgeUnexpectedChange).not.toHaveBeenCalled();
    expect(props.onOpenChange).toHaveBeenCalledExactlyOnceWith(false);
  });

  it("opens the text pilot menu for the owning controller", () => {
    render(dialog);

    fireEvent.click(screen.getByRole("button", { name: point }));

    expect(state.openTextPilotMenu).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({
      ...draft,
      callsign: "SAS123",
      point,
      actions: expect.arrayContaining([
        { type: ActionType.FrontendReleasePoint, callsign: "SAS123", release_point: point },
        { type: ActionType.FrontendAcknowledgeUnexpectedChange, callsign: "SAS123", field_name: "release_point" },
      ]),
    }));
    expect(state.setReleasePoint).not.toHaveBeenCalled();
    expect(state.move).not.toHaveBeenCalled();
    expect(state.acknowledgeUnexpectedChange).not.toHaveBeenCalled();
    expect(props.onOpenChange).not.toHaveBeenCalled();
  });
});
