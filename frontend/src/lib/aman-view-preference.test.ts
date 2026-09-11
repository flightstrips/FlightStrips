import {beforeEach, describe, expect, it} from "vitest";

import type {AMANState} from "@/api/aman";
import {
  AMAN_ALL_VIEW,
  AMAN_VIEW_PREFERENCE_KEY,
  readAMANViewPreference,
  resolveAMANView,
  writeAMANViewPreference,
} from "./aman-view-preference";

const state = {
  timeline_configuration: {
    version: "mapping-v1",
    mappings: [{id: 1, left: "NORTH", right: "EAST"}, {id: 2, left: "SOUTH", right: null}],
  },
} as AMANState;

describe("local AMAN view preference", () => {
  beforeEach(() => localStorage.clear());

  it("stores only the selected view and restores it when the server still offers it", () => {
    writeAMANViewPreference("EAST");
    expect(JSON.parse(localStorage.getItem(AMAN_VIEW_PREFERENCE_KEY)!)).toEqual({version: 1, view: "EAST"});
    expect(resolveAMANView(state, readAMANViewPreference())).toBe("EAST");
  });

  it("defaults zero or multiple controller matches to ALL and accepts exactly one available match", () => {
    expect(resolveAMANView(state, null, [])).toBe(AMAN_ALL_VIEW);
    expect(resolveAMANView(state, null, ["NORTH", "EAST"])).toBe(AMAN_ALL_VIEW);
    expect(resolveAMANView(state, null, ["retired", "SOUTH"])).toBe("SOUTH");
  });

  it("tolerates old servers and stale or malformed local preferences", () => {
    localStorage.setItem(AMAN_VIEW_PREFERENCE_KEY, JSON.stringify({version: 1, view: "retired"}));
    expect(resolveAMANView(state, readAMANViewPreference())).toBe(AMAN_ALL_VIEW);
    expect(resolveAMANView({...state, timeline_configuration: undefined}, "NORTH")).toBe(AMAN_ALL_VIEW);
    localStorage.setItem(AMAN_VIEW_PREFERENCE_KEY, "{");
    expect(readAMANViewPreference()).toBeNull();
  });
});
