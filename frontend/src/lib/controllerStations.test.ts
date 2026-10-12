import { describe, expect, it } from "vitest";
import type { FrontendController } from "@/api/models";
import { findStationController } from "./controllerStations";

const controller: FrontendController = { callsign: "EKCH_A_TWR", position: "118.105", identifier: "TW", section: "TWR", owned_sectors: ["TW", "TE", "GWD"] };
describe("station availability", () => {
  it("finds direct and cross-coupled stations, normalizing ground west", () => {
    for (const station of [{ frequency: "118.105", sectors: ["TW"] }, { frequency: "119.355", sectors: ["TE"] }, { frequency: "118.580", sectors: ["GW"] }]) {
      expect(findStationController(station, [controller], "119.905", "EKCH_DEL")).toBe(controller);
    }
  });
  it("excludes own station, own callsign, and observers", () => {
    const station = { frequency: "118.105", sectors: ["TW"] };
    expect(findStationController(station, [controller], "118.105", "OTHER")).toBeUndefined();
    expect(findStationController(station, [controller], "119.905", "ekch_a_twr")).toBeUndefined();
    expect(findStationController(station, [{ ...controller, observer: true }], "", "")).toBeUndefined();
    expect(findStationController(station, [{ ...controller, callsign: "EKCH_OBS" }], "", "")).toBeUndefined();
  });
});
