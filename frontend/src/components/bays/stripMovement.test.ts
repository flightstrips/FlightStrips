import { describe, expect, it } from "vitest";
import { Bay, type FrontendStrip, type TacticalStrip } from "@/api/models";
import { allBayTransferRules, canStripMoveToBay, isArrivalFlight } from "./stripMovement";

const flight = (origin: string, destination: string) => ({
  callsign: "TEST123",
  origin,
  destination,
} as FrontendStrip);

const tactical = {
  id: 42,
  type: "MEMAID",
} as TacticalStrip;

describe("strip bay movement eligibility", () => {
  it("classifies only inbound flights as arrivals", () => {
    expect(isArrivalFlight(flight("ESSA", "EKCH"), "EKCH")).toBe(true);
    expect(isArrivalFlight(flight("EKCH", "ESSA"), "EKCH")).toBe(false);
    expect(isArrivalFlight(flight("EKCH", "EKCH"), "EKCH")).toBe(false);
  });

  it("allows departures and other flights into NOT CLEARED", () => {
    expect(canStripMoveToBay(flight("EKCH", "ESSA"), Bay.NotCleared, "EKCH")).toBe(true);
    expect(canStripMoveToBay(flight("EKCH", "EKCH"), Bay.NotCleared, "EKCH")).toBe(true);
  });

  it("rejects arrivals and tactical strips only for NOT CLEARED", () => {
    const arrival = flight("ESSA", "EKCH");
    expect(canStripMoveToBay(arrival, Bay.NotCleared, "EKCH")).toBe(false);
    expect(canStripMoveToBay(tactical, Bay.NotCleared, "EKCH")).toBe(false);
    expect(canStripMoveToBay(arrival, Bay.Cleared, "EKCH")).toBe(true);
    expect(canStripMoveToBay(arrival, Bay.Depart, "EKCH")).toBe(true);
    expect(canStripMoveToBay(tactical, Bay.Final, "EKCH")).toBe(true);
  });

  it("connects every registered visual bay to every other bay", () => {
    expect(allBayTransferRules(["FINAL", "RWY-ARR", "CLRDEL"])).toEqual({
      "FINAL": ["RWY-ARR", "CLRDEL"],
      "RWY-ARR": ["FINAL", "CLRDEL"],
      "CLRDEL": ["FINAL", "RWY-ARR"],
    });
  });
});
