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
import type { HalfStripVariant, StripProps, StripStatus } from "./types";

export type { StripStatus };
export type { StripProps };
export type { HalfStripVariant };

interface StripRenderProps {
  strip: AnyStrip;
  status?: StripStatus;
  halfStripVariant?: HalfStripVariant;
  myPosition?: string;
  selectable?: boolean;
  width?: number | string;
  fullWidth?: boolean;
}

export function Strip({ strip, status, halfStripVariant, myPosition, selectable, width, fullWidth }: StripRenderProps) {
  if (!isFlight(strip)) {
    switch (strip.type) {
      case "MEMAID":
        return <TacticalMemaidStrip strip={strip} width={width} />;
      case "CROSSING":
        return <TacticalCrossingStrip strip={strip} width={width} />;
      case "START":
      case "LAND":
        return <TacticalRwyStrip strip={strip} width={width} />;
      default:
        return null;
    }
  }

  const props: StripProps = {
    callsign: strip.callsign,
    bay: strip.bay as Bay,
    pdcStatus: strip.pdc_state ?? ("NONE" as PdcStatus),
    destination: strip.destination,
    origin: strip.origin,
    stand: strip.stand,
    eobt: strip.eobt,
    tobt: strip.tobt,
    tsat: strip.tsat,
    ctot: strip.ctot,
    aircraftType: strip.aircraft_type,
    squawk: strip.squawk,
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
    halfStripVariant,
    myPosition,
    selectable,
    marked: strip.marked,
    runwayCleared: strip.runway_cleared,
    registration: strip.registration,
    fullWidth,
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
