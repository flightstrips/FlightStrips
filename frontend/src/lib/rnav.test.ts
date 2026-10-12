import { describe, expect, it } from "vitest";
import type { ClxValidationFault } from "@/api/models";
import { buildRnavUpdate, effectiveClxFaults, effectiveRnavCapability, rnavAssumptionRemark } from "./rnav";

describe("effective RNAV capability", () => {
  it.each([
    "A320", "A359", "A3N9", "B736", "B737", "B738", "B739", "B38M",
    "B744", "B748", "B752", "B753", "B762", "B763", "B764", "B77W",
    "B789", "BCS3", "AT72", "AT8A", "DH8D", "CRJ9", "E190", "E75L", "MD82", "MD88", "MD8A",
  ])("assumes RNAV 1 for %s regardless of filed capability", (aircraft) => {
    for (const remarks of ["", "PBN/A1", "PBN/B1", "PBN/C1", "PBN/D1"]) {
      expect(effectiveRnavCapability(`${aircraft}/M-R/LB1`, remarks)).toBe("1");
    }
    expect(effectiveRnavCapability(aircraft, "")).toBe("1");
  });

  describe("effective clearance faults", () => {
    const rnavFault: ClxValidationFault = { code: "RNAV_NIL", message: "No RNAV", nitos_remark: "NON RNAV", fields: ["rnav"] };
    const sidFault: ClxValidationFault = { code: "SID", message: "Invalid SID", nitos_remark: "CHECK SID", fields: ["sid"] };

    it.each(["A320", "B738", "AT72", "DH8D", "CRJ9", "E190"])("removes stale RNAV warnings for %s without hiding other faults", (aircraft_type) => {
      expect(effectiveClxFaults({ aircraft_type, remarks: "", clx_validation: { faults: [rnavFault, sidFault] } })).toEqual([sidFault]);
    });

    describe("RNAV assumption internal remark", () => {
      it.each(["", "PBN/A1", "PBN/B1", "PBN/C1"])("explains the aircraft-type correction for filed %s", (remarks) => {
        expect(rnavAssumptionRemark("B738/M-R", remarks)).toBe(
          "Aircraft filed without RNAV capability, but corrected based on A/C Type. Consider verification if deemed relevant.",
        );
      });

      it("does not add a note when RNAV 1 is actually filed", () => {
        expect(rnavAssumptionRemark("B738/M-R", "PBN/D1")).toBe("");
      });

      it("does not add a note for aircraft without the type-based correction", () => {
        expect(rnavAssumptionRemark("C172/L-S", "")).toBe("");
        expect(rnavAssumptionRemark("C172/L-R", "PBN/D1")).toBe("");
      });
    });

    it("clears an obsolete RNAV warning immediately after REMOVE NON RNAV", () => {
      const update = buildRnavUpdate("C172/L-S", "", "1");
      expect(effectiveClxFaults({ ...update, clx_validation: { faults: [rnavFault] } })).toEqual([]);
    });

    it("retains RNAV warnings for non-exempt aircraft without RNAV 1", () => {
      expect(effectiveClxFaults({ aircraft_type: "C172/L-R", remarks: "PBN/B1", clx_validation: { faults: [rnavFault] } })).toEqual([rnavFault]);
    });

    it("preserves faults that also describe other fields", () => {
      const mixedFault: ClxValidationFault = { ...sidFault, fields: ["sid", "rnav"] };
      expect(effectiveClxFaults({ aircraft_type: "B738", remarks: "", clx_validation: { faults: [mixedFault] } })).toEqual([mixedFault]);
    });

    it("handles absent validation", () => {
      expect(effectiveClxFaults(undefined)).toEqual([]);
      expect(effectiveClxFaults({ aircraft_type: "B738", remarks: "" })).toEqual([]);
    });
  });

  it.each(["A20N", "B735", "B734", "B743", "B741", "B761", "BCS1", "DH8C", "C172", "A3200", "AT7", "CRJ", "E19", "E1900", "E!90", "MD90", "MD8", "MD820"])("uses filed capability for %s", (aircraft) => {
    expect(effectiveRnavCapability(aircraft, "")).toBe("NIL");
    expect(effectiveRnavCapability(`${aircraft}/M-R`, "PBN/C1")).toBe("2");
  });

  it("normalizes lowercase aircraft codes", () => {
    expect(effectiveRnavCapability(" b738/m-s ", "")).toBe("1");
  });

  it("REMOVE NON RNAV replaces PBN, adds R, and preserves unrelated remarks", () => {
    expect(buildRnavUpdate("C172/L-S", "REG/OYABC PBN/A1 RMK/TEST", "1")).toEqual({
      aircraft_type: "C172/L-SR",
      remarks: "REG/OYABC PBN/A1B1C1D1S1S2 RMK/TEST",
      capabilities: "1",
    });
  });
});
