import { describe, expect, it } from "vitest";

import { isEstDepartureTransferActive } from "./transferState";

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
