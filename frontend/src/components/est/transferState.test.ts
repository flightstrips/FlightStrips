import { describe, expect, it } from "vitest";

import { getEstDepartureTransferTarget, isEstDepartureTransferActive, isEstDepartureTransferred } from "./transferState";

describe("isEstDepartureTransferActive", () => {
  it("recognizes the active START REQ transfer to APRON", () => {
    expect(
      isEstDepartureTransferActive(
        { from: "EKCH_DEL", to: "EKCH_A_GND", isTagRequest: false },
        "EKCH_DEL",
        "EKCH_A_GND",
      ),
    ).toBe(true);
  });

  it("does not treat a tag request as a START REQ transfer", () => {
    expect(
      isEstDepartureTransferActive(
        { from: "EKCH_DEL", to: "EKCH_A_GND", isTagRequest: true },
        "EKCH_DEL",
        "EKCH_A_GND",
      ),
    ).toBe(false);
  });

  it("rejects transfers from another position or to another target", () => {
    expect(
      isEstDepartureTransferActive(
        { from: "EKCH_B_GND", to: "EKCH_A_GND", isTagRequest: false },
        "EKCH_DEL",
        "EKCH_A_GND",
      ),
    ).toBe(false);
    expect(
      isEstDepartureTransferActive(
        { from: "EKCH_DEL", to: "EKCH_TWR", isTagRequest: false },
        "EKCH_DEL",
        "EKCH_A_GND",
      ),
    ).toBe(false);
  });
});

describe("getEstDepartureTransferTarget", () => {
  it("uses the strip's first next controller instead of a hard-coded sector owner", () => {
    expect(
      getEstDepartureTransferTarget(
        { next_controllers: ["121.905", "121.730", "118.580"] },
        "121.905",
      ),
    ).toBe("121.730");
  });

  describe("isEstDepartureTransferred", () => {
    it("uses persisted ownership history after acceptance, including on reconnect", () => {
      expect(isEstDepartureTransferred({
        owner: "121.730",
        previous_controllers: ["121.905"],
      }, "121.905")).toBe(true);
    });

    it("does not hide CDM for unrelated, unowned or reacquired strips", () => {
      expect(isEstDepartureTransferred({ owner: "121.730", previous_controllers: [] }, "121.905")).toBe(false);
      expect(isEstDepartureTransferred({ owner: "121.730", previous_controllers: null }, "121.905")).toBe(false);
      expect(isEstDepartureTransferred({ owner: "", previous_controllers: ["121.905"] }, "121.905")).toBe(false);
      expect(isEstDepartureTransferred({ owner: "121.905", previous_controllers: ["121.905"] }, "121.905")).toBe(false);
      expect(isEstDepartureTransferred({ owner: "121.730", previous_controllers: ["121.905"] }, "")).toBe(false);
      expect(isEstDepartureTransferred(undefined, "121.905")).toBe(false);
    });
  });

  it("returns no target when the route only contains the current primary", () => {
    expect(
      getEstDepartureTransferTarget({ next_controllers: ["121.905"] }, "121.905"),
    ).toBe("");
  });
});
