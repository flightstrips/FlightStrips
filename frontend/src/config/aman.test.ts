import {describe, expect, it} from "vitest";

import {EKCH_TMT_HOLDING_ORDER, holdingsForFamilies} from "./aman";

describe("holdingsForFamilies", () => {
  it("shows every holding when no STAR is selected", () => {
    expect(holdingsForFamilies([])).toEqual(EKCH_TMT_HOLDING_ORDER);
  });

  it("shows only holdings of the selected STARs in display order", () => {
    expect(holdingsForFamilies(["TESPI", "TIDVU"])).toEqual(["TIDVU", "ROSBI"]);
    expect(holdingsForFamilies(["MONAK"])).toEqual(["OLPIB"]);
  });
});
