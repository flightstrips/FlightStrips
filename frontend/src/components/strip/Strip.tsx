import type { AnyStrip, PdcStatus } from "@/api/models";
import { Bay, isFlight } from "@/api/models";
import { ApnArrStrip } from "./ApnArrStrip";
import { ApnPushStrip } from "./ApnPushStrip";
import { ApnTaxiDepStrip } from "./ApnTaxiDepStrip";
import { ClxClearedStrip } from "./ClxClearedStrip";
import { DelStrip } from "./DelStrip";
import { FinalArrStrip } from "./FinalArrStrip";
import { HalfStrip } from "./HalfStrip";
import { TwyDepStrip } from "./TwyDepStrip";
import { ClxHalfStrip } from "./ClxHalfStrip";
import { TacticalMemaidStrip } from "./TacticalMemaidStrip";
import { TacticalCrossingStrip } from "./TacticalCrossingStrip";
import { TacticalRwyStrip } from "./TacticalRwyStrip";
import { ControlzoneStrip } from "./ControlzoneStrip";
import type { HalfStripVariant, StripProps, StripStatus } from "./types";
import { normalizeCdmTime } from "@/lib/cdmTime";

export type { StripStatus };
export type { StripProps };
export type { HalfStripVariant };

interface StripRenderProps {
  strip: AnyStrip;
  status?: StripStatus;
  halfStripVariant?: HalfStripVariant;
  myPosition?: string;
  selectable?: boolean;
  delegateCallsignClick?: boolean;
  onStripMoved?: () => void;
  width?: number | string;
  fullWidth?: boolean;
}

// Maps each strip status to the internal width used by the corresponding flight strip component.
// Used so tactical strips (memaid, crossing, etc.) match the width of flight strips in the same bay.
const STATUS_DEFAULT_WIDTH: Partial<Record<StripStatus, string>> = {
  "ARR":      "90%",   // ApnArrStrip
  "FINAL-ARR":"95%",   // FinalArrStrip
  "PUSH":     "90%",   // ApnPushStrip (non-fullWidth)
  "TWY-DEP":  "95%",   // TwyDepStrip
  "TAXI-DEP": "90%",   // ApnTaxiDepStrip
  "CLR":      "80%",   // DelStrip (non-fullWidth)
  "CLX-HALF": "80%",   // ClxHalfStrip (non-fullWidth)
  "CLROK":    "88.44%",// ClxClearedStrip (non-fullWidth)
};

export function Strip({ strip, status, halfStripVariant, myPosition, selectable, delegateCallsignClick, onStripMoved, width, fullWidth }: StripRenderProps) {
  if (!isFlight(strip)) {
    const effectiveWidth = width ?? (status ? STATUS_DEFAULT_WIDTH[status] : undefined);
    switch (strip.type) {
      case "MEMAID":
        return <TacticalMemaidStrip strip={strip} width={effectiveWidth} />;
      case "CROSSING":
        return <TacticalCrossingStrip strip={strip} width={effectiveWidth} />;
      case "START":
      case "LAND":
        return <TacticalRwyStrip strip={strip} width={effectiveWidth} />;
      default:
        return null;
    }
  }

  if (status === "CONTROLZONE") {
    return <ControlzoneStrip strip={strip} selectable={selectable} />;
  }

  const props: StripProps = {
    callsign: strip.callsign,
    bay: strip.bay as Bay,
    pdcStatus: strip.pdc_state ?? ("NONE" as PdcStatus),
    destination: strip.destination,
    origin: strip.origin,
    stand: strip.stand,
    eobt: normalizeCdmTime(strip.eobt),
    tobt: normalizeCdmTime(strip.tobt),
    reqTobtType: strip.req_tobt_type,
    tobtSetBy: strip.tobt_set_by,
    tsat: normalizeCdmTime(strip.tsat),
    ctot: normalizeCdmTime(strip.ctot),
    phase: strip.phase,
    aircraftType: strip.aircraft_type,
    aircraftCategory: strip.aircraft_category,
    squawk: strip.squawk,
    assignedSquawk: strip.assigned_squawk,
    sid: strip.sid,
    runway: strip.runway,
    clearedAltitude: strip.cleared_altitude,
    requestedAltitude: strip.requested_altitude,
    heading: strip.heading,
    holdingPoint: strip.release_point,
    taxiway: strip.release_point,
    owner: strip.owner,
    nextControllers: strip.next_controllers,
    previousControllers: strip.previous_controllers,
    nextDisplay: strip.next_display,
    halfStripVariant,
    myPosition,
    selectable,
    delegateCallsignClick,
    onStripMoved,
    marked: strip.marked,
    runwayCleared: strip.runway_cleared,
    runwayConfirmed: strip.runway_confirmed,
    registration: strip.registration,
    fullWidth,
    unexpectedChangeFields: strip.unexpected_change_fields,
    controllerModifiedFields: strip.controller_modified_fields,
    isManual: strip.is_manual,
    validationStatus: strip.validation_status,
    ecfmp_restrictions: strip.ecfmp_restrictions,
    requested_altitude: strip.requested_altitude,
  };

  switch (status) {
    case "CLR":
      return <DelStrip {...props} />;
    case "CLROK":
      return <ClxClearedStrip {...props} />;
    case "HALF":
      return <HalfStrip {...props} />;
    case "PUSH":
      return <ApnPushStrip {...props} />;
    case "ARR":
      return <ApnArrStrip {...props} />;
    case "FINAL-ARR":
      return <FinalArrStrip {...props} />;
    case "CLX-HALF":
      return <ClxHalfStrip {...props} />;
    case "TAXI-DEP":
      return <ApnTaxiDepStrip {...props} />;
    case "TWY-DEP":
      return <TwyDepStrip {...props} />;
    default:
      return null;
  }
}
