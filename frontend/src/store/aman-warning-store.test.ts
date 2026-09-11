import {describe, expect, it} from "vitest";

import type {AMANState, AMANWarning} from "@/api/aman";
import {currentWarningsFromAMANState} from "./aman-warning-store";

const error: AMANWarning = {
  id: "warning:error-b", source: "technical_health", component: "repository",
  severity: "error", code: "blocked", message: "Repository blocked",
};
const warning: AMANWarning = {
  id: "warning:warning-a", source: "technical_health", component: "weather",
  severity: "warning", code: "stale", message: "Weather stale",
};

function state(warnings?: AMANWarning[]): AMANState {
  return {warnings} as unknown as AMANState;
}

describe("current AMAN warning replacement", () => {
  it("deduplicates by stable identity and orders errors before warnings then identity", () => {
    const earlierError = {...error, id: "warning:error-a", message: "Earlier error"};
    expect(currentWarningsFromAMANState(state([warning, error, earlierError, error])).items)
      .toEqual([earlierError, error, warning]);
  });

  it("clears prior content for empty and omitted compatible replacements", () => {
    expect(currentWarningsFromAMANState(state([]))).toEqual({items: [], snapshot: "available"});
    expect(currentWarningsFromAMANState(state())).toEqual({items: [], snapshot: "omitted"});
    expect(currentWarningsFromAMANState(null)).toEqual({items: [], snapshot: "omitted"});
  });
});
