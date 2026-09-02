import { describe, expect, it } from "vitest";
import { Bay, type FrontendStrip, type TacticalStrip } from "@/api/models";
import { selectFlightStripsForBay, selectStripsForBay } from "./ekch";

const flight = (callsign: string, origin: string, destination: string, bay: Bay) => ({
  callsign,
  origin,
  destination,
  bay,
} as FrontendStrip);

describe("EKCH bay selectors", () => {
  it("keeps an arrival visible in a departure bay", () => {
    const arrival = flight("ARR123", "ESSA", "EKCH", Bay.Taxi);
    expect(selectFlightStripsForBay([arrival], Bay.Taxi)).toEqual([arrival]);
  });

  it("keeps a departure visible in an arrival bay", () => {
    const departure = flight("DEP123", "EKCH", "ESSA", Bay.TwyArr);
    expect(selectFlightStripsForBay([departure], Bay.TwyArr)).toEqual([departure]);
  });

  it("keeps tactical strips visible in the cleared/startup bay", () => {
    const tactical = {
      id: 42,
      type: "MEMAID",
      bay: Bay.Cleared,
      sequence: 1000,
    } as TacticalStrip;

    expect(selectStripsForBay([], [tactical], Bay.Cleared)).toEqual([tactical]);
  });
});
