import { describe, expect, it } from "vitest";
import { defaultTextSelections, textPilotPhrases, taxiLimit, surfaceWind, togglePhraseText, isTextPilot, stationController } from "./text-pilot";
import { CommunicationType, type FrontendController } from "@/api/models";

describe("text pilot phrase rules", () => {
  it("changes behavior only for full text pilots", () => {
    expect(isTextPilot({ communication_type: CommunicationType.Text })).toBe(true);
    expect(isTextPilot({ communication_type: CommunicationType.Receive })).toBe(false);
    expect(isTextPilot(undefined)).toBe(false);
  });
  it("formats hold-short, assigned stand and holding-point limits", () => {
    expect(taxiLimit({ callsign: "SAS123", variant: "apron-out", point: "A/B" }, "22R")).toBe("TWY A, Hold short of TWY B.");
    expect(taxiLimit({ callsign: "SAS123", variant: "apron-out", point: "K2" }, "22R")).toBe("TWY K2, Hold short RWY 12/30.");
    expect(taxiLimit({ callsign: "SAS123", variant: "tower-out", point: "A1", holdingPoint: true }, "22R")).toBe("for Holding point A1, Runway 22R.");
    expect(taxiLimit({ callsign: "SAS123", variant: "apron-in", stand: "A18" }, "22R")).toBe("to stand A18.");
  });
  it("uses tower split defaults only in split layouts", () => {
    const draft = { callsign: "SAS123", variant: "tower-in" as const, point: "K3" };
    expect(defaultTextSelections(draft)).toEqual(["taxi", "limit"]);
    expect(defaultTextSelections({ ...draft, splitTowerDefaults: true })).toEqual(["taxi", "rwy:30", "twy:K3", "hold-k3"]);
  });
  it("highlights apron runway hold-short components individually", () => {
    expect(defaultTextSelections({ callsign: "SAS123", variant: "apron-out", point: "K2" })).toEqual(["taxi", "twy:K2", "hold-12"]);
  });
  it.each([[false,false,"lineup"],[false,true,"takeoff"],[true,false,"continue-approach"],[true,true,"land"]])("selects accepted runway state arrival=%s confirmed=%s", (arrival,confirmed,instruction) => {
    expect(defaultTextSelections({ callsign: "SAS123", variant: "runway", arrival: arrival as boolean, confirmed: confirmed as boolean })).toEqual(confirmed ? ["wind",instruction] : [instruction]);
  });
  it("uses recorded wind and disables an unavailable wind", () => {
    expect(surfaceWind("EKCH 101220Z 24012KT CAVOK 12/10 Q1013")).toBe("Wind 240 degrees, 12 knots.");
    expect(surfaceWind("EKCH 101220Z VRB03KT CAVOK 12/10 Q1013")).toBe("Wind variable, 3 knots.");
    expect(textPilotPhrases({ callsign: "SAS123", variant: "runway" }, { runway: "22R" }, "")[0].disabled).toBe(true);
  });
  it("removes only the last generated occurrence and retains manual edits", () => {
    expect(togglePhraseText("Custom. TWY A, TWY A,", "TWY A,", true)).toBe("Custom. TWY A,");
    expect(togglePhraseText("Custom edit", "TWY A,", true)).toBe("Custom edit");
  });
  it("resolves cross-coupled stations without selecting observers", () => {
    const controllers = [{ position: "118.105", callsign: "EKCH_TWR", owned_sectors: ["SEQ"] }, { position: "121.905", callsign: "EKCH_OBS", observer: true }] as FrontendController[];
    expect(stationController("121.905", controllers)?.position).toBe("118.105");
  });
});
