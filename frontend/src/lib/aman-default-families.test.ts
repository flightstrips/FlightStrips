import {describe, expect, it} from "vitest";

import {defaultACCFamilies} from "./aman-default-families";

const ALL = new Set(["TESPI", "TUDLO", "MONAK", "TIDVU", "ERNOV"]);
const online = (...callsigns: string[]) => callsigns.map((callsign) => ({callsign}));
const sorted = (values: string[]) => [...values].sort();

describe("defaultACCFamilies", () => {
  it("selects MONAK for B and adds TUDLO/TESPI when D, E or I is missing", () => {
    expect(defaultACCFamilies("EKDK_B_CTR", online("EKDK_D_CTR", "EKDK_E_CTR", "EKDK_I_CTR"), ALL)).toEqual(["MONAK"]);
    expect(sorted(defaultACCFamilies("EKDK_B_CTR", online("EKDK_D_CTR"), ALL))).toEqual(["MONAK", "TESPI", "TUDLO"]);
  });

  it("covers missing neighbours for I, D and E", () => {
    expect(sorted(defaultACCFamilies("EKDK_I_CTR", online(), ALL))).toEqual(["MONAK", "TESPI", "TUDLO"]);
    expect(defaultACCFamilies("EKDK_I_CTR", online("EKDK_B_CTR", "EKDK_D_CTR", "EKDK_E_CTR"), ALL)).toEqual([]);
    expect(sorted(defaultACCFamilies("EKDK_D_CTR", online("EKDK_B_CTR", "EKDK_I_CTR"), ALL))).toEqual(["TESPI", "TUDLO"]);
    expect(sorted(defaultACCFamilies("EKDK_D_CTR", online("EKDK_E_CTR"), ALL))).toEqual(["MONAK", "TUDLO"]);
    expect(defaultACCFamilies("EKDK_E_CTR", online("EKDK_D_CTR"), ALL)).toEqual(["TESPI"]);
    expect(sorted(defaultACCFamilies("EKDK_E_CTR", online(), ALL))).toEqual(["MONAK", "TESPI", "TUDLO"]);
  });

  it("does not count observers as online", () => {
    expect(sorted(defaultACCFamilies("EKDK_D_CTR", [{callsign: "EKDK_E_CTR", observer: true}, {callsign: "EKDK_B_CTR"}, {callsign: "EKDK_I_CTR"}], ALL))).toEqual(["TESPI", "TUDLO"]);
  });

  it("applies the Swedish position rules", () => {
    for (const position of ["ESMS_APP", "ESMM_7_CTR", "ESMM_8_CTR", "ESMM_Y_CTR"]) expect(defaultACCFamilies(position, [], ALL)).toEqual(["TIDVU"]);
    expect(defaultACCFamilies("ESMM_5_CTR", [], ALL)).toEqual(["ERNOV"]);
    expect(sorted(defaultACCFamilies("ESMM_K_CTR", [], ALL))).toEqual(["ERNOV", "TIDVU"]);
    expect(defaultACCFamilies("ESMM_K_CTR", online("ESMM_7_CTR"), ALL)).toEqual(["ERNOV"]);
    expect(sorted(defaultACCFamilies("ESMM_2_CTR", [], ALL))).toEqual(["ERNOV", "TIDVU"]);
    expect(defaultACCFamilies("ESMM_2_CTR", online("ESMS_APP"), ALL)).toEqual(["ERNOV"]);
    expect(defaultACCFamilies("ESMM_2_CTR", online("ESMM_K_CTR"), ALL)).toEqual(["TIDVU"]);
    expect(defaultACCFamilies("ESMM_2_CTR", online("ESMM_Y_CTR", "ESMM_5_CTR"), ALL)).toEqual([]);
  });

  it("applies fixed sets for upper-area and approach positions", () => {
    expect(sorted(defaultACCFamilies("EKDK_UC_CTR", [], ALL))).toEqual(["MONAK", "TESPI", "TUDLO"]);
    expect(sorted(defaultACCFamilies("EKCH_W_APP", [], ALL))).toEqual(["ERNOV", "TESPI", "TUDLO"]);
    expect(sorted(defaultACCFamilies("EKCH_O_APP", [], ALL))).toEqual(["MONAK", "TIDVU"]);
    expect(defaultACCFamilies("EKCH_P1_APP", [], ALL)).toEqual([]);
    expect(defaultACCFamilies("EKCH_TWR", [], ALL)).toEqual([]);
  });
});
