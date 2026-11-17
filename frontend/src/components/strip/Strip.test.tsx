import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Bay, type FrontendStrip } from "@/api/models";
import { Strip } from "./Strip";

const stripState = vi.hoisted(() => ({ locallyHidden: true }));

vi.mock("@/store/store-hooks", () => ({
  useAirport: () => "EKCH",
  useWebSocketStore: (selector: (state: { localMarked: Record<string, boolean> }) => unknown) =>
    selector({ localMarked: {} }),
}));

vi.mock("@/store/localHiddenStrips", () => ({
  useIsLocallyHidden: () => stripState.locallyHidden,
}));

vi.mock("./HalfStrip", () => ({ HalfStrip: () => <div data-testid="half-strip" /> }));

describe("Strip local hiding", () => {
  beforeEach(() => {
    stripState.locallyHidden = true;
  });

  it("completely removes locally hidden arrivals", () => {
    const arrival = {
      callsign: "ARR123",
      origin: "ESSA",
      destination: "EKCH",
      bay: Bay.Final,
    } as FrontendStrip;

    const { container } = render(<Strip strip={arrival} status="ARR" />);

    expect(container.firstChild).toBeNull();
  });

  it("keeps locally hidden departures available as half strips", () => {
    const departure = {
      callsign: "DEP123",
      origin: "EKCH",
      destination: "ESSA",
      bay: Bay.Push,
    } as FrontendStrip;

    render(<Strip strip={departure} status="PUSH" />);

    expect(screen.getByTestId("half-strip")).toBeInTheDocument();
  });
});
