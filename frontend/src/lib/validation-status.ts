import type { ValidationStatus } from "@/api/models";

const PDC_VALIDATION_ISSUE_TYPES = new Set(["PDC INVALID", "CUSTOM PDC"]);
const NON_BLOCKING_VALIDATION_ISSUE_TYPES = new Set(["STAND ASSIGNMENT"]);

const OPEN_DCL_MENU_ALLOWED_FIELDS = new Set<ValidationEditableField>([
  "sid",
  "runway",
  "route",
  "eobt",
  "capabilities",
]);

export type ValidationEditableField =
  | "sid"
  | "eobt"
  | "route"
  | "heading"
  | "altitude"
  | "stand"
  | "remarks"
  | "aircraft_type"
  | "capabilities"
  | "runway";

export type ValidationAttemptedAction =
  | { type: "move" }
  | { type: "generate_squawk" }
  | { type: "cdm_ready" }
  | { type: "update_order" }
  | { type: "update_strip"; fields: ValidationEditableField[] }
  | { type: "release_point" }
  | { type: "coordination_transfer_request" }
  | { type: "coordination_assume_request" }
  | { type: "coordination_free_request" }
  | { type: "coordination_tag_request" }
  | { type: "coordination_accept_tag_request" }
  | { type: "runway_clearance" }
  | { type: "runway_confirmation" };

export function isPdcValidationStatus(validationStatus: ValidationStatus | undefined): boolean {
  return validationStatus != null && PDC_VALIDATION_ISSUE_TYPES.has(validationStatus.issue_type);
}

export function isValidationActiveForPosition(validationStatus: ValidationStatus | undefined, myPosition?: string): boolean {
  return validationStatus?.active === true
    && (isPdcValidationStatus(validationStatus) || validationStatus.owning_position === myPosition);
}

export function isValidationBlockingForPosition(validationStatus: ValidationStatus | undefined, myPosition?: string): boolean {
  return isValidationActiveForPosition(validationStatus, myPosition)
    && !NON_BLOCKING_VALIDATION_ISSUE_TYPES.has(validationStatus?.issue_type ?? "");
}

function hasCustomAction(validationStatus: ValidationStatus | undefined, actionKind: string): boolean {
  return validationStatus?.custom_action?.action_kind === actionKind;
}

function areUpdateFieldsAllowed(validationStatus: ValidationStatus | undefined, fields: ValidationEditableField[]): boolean {
  if (fields.length === 0) {
    return false;
  }

  if (hasCustomAction(validationStatus, "assign_stand")) {
    return fields.every((field) => field === "stand");
  }

  if (hasCustomAction(validationStatus, "open_dcl_menu")) {
    return fields.every((field) => OPEN_DCL_MENU_ALLOWED_FIELDS.has(field));
  }

  return false;
}

export function isValidationActionAllowed(
  validationStatus: ValidationStatus | undefined,
  action: ValidationAttemptedAction,
): boolean {
  if (!validationStatus?.active || NON_BLOCKING_VALIDATION_ISSUE_TYPES.has(validationStatus.issue_type)) {
    return true;
  }

  switch (action.type) {
    case "generate_squawk":
      return hasCustomAction(validationStatus, "generate_squawk");
    case "cdm_ready":
      return true;
    case "release_point":
      return hasCustomAction(validationStatus, "assign_holding_point");
    case "runway_clearance":
      return hasCustomAction(validationStatus, "runway_clearance");
    case "update_strip":
      return areUpdateFieldsAllowed(validationStatus, action.fields);
    default:
      return false;
  }
}
