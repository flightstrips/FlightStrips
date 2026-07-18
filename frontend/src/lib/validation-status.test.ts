import { describe, expect, it } from "vitest";
import type { ValidationStatus } from "@/api/models";
import {
  isValidationActionAllowed,
  isValidationActiveForPosition,
  isValidationBlockingForPosition,
} from "./validation-status";

const status = (issueType: string, active = true): ValidationStatus => ({
  issue_type: issueType,
  message: "test",
  owning_position: "EKCH_DEL",
  active,
  activation_key: "key",
});

describe("validation status blocking", () => {
  it("keeps stand assignment advisories active without locking the strip", () => {
    const advisory = status("STAND ASSIGNMENT");

    expect(isValidationActiveForPosition(advisory, "EKCH_DEL")).toBe(true);
    expect(isValidationBlockingForPosition(advisory, "EKCH_DEL")).toBe(false);
    expect(isValidationActionAllowed(advisory, { type: "move" })).toBe(true);
    expect(isValidationActionAllowed(advisory, { type: "coordination_transfer_request" })).toBe(true);
    expect(isValidationActionAllowed(advisory, {
      type: "update_strip",
      fields: ["sid", "route", "runway", "eobt"],
    })).toBe(true);
  });

  it("continues to lock active operational validations", () => {
    const validation = status("WRONG SQUAWK");

    expect(isValidationBlockingForPosition(validation, "EKCH_DEL")).toBe(true);
    expect(isValidationActionAllowed(validation, { type: "move" })).toBe(false);
  });

  it("does not expose owner-scoped validation locks to other positions", () => {
    expect(isValidationBlockingForPosition(status("WRONG SQUAWK"), "EKCH_GND")).toBe(false);
  });
});
