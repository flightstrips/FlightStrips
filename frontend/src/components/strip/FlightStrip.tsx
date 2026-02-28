import type { PdcStatus } from "@/api/models";
import { ApnArrStrip } from "./ApnArrStrip";
import { ApnPushStrip } from "./ApnPushStrip";
import { ApnTaxiDepStrip } from "./ApnTaxiDepStrip";
import { ClxClearedStrip } from "./ClxClearedStrip";
import { DelStrip } from "./DelStrip";
import { HalfStrip } from "./HalfStrip";
import type { HalfStripVariant, StripProps, StripStatus } from "./types";
import { ClxHalfStrip } from "./ClxHalfStrip";

export type { StripStatus };
export type { StripProps };
export type { HalfStripVariant };

export interface FlightStripProps extends StripProps {
  status?: StripStatus;
}

/**
 * FlightStrip – top-level strip dispatcher.
 *
 *  - `"CLR"`      → DelStrip        (pre-clearance / UNCLEARED bays)
 *  - `"CLROK"`   → GroundStrip     (ground movement / TWY DEP)
 *  - `"HALF"`    → HalfStrip       (21px compact — FINAL locked strips)
 *  - `"PUSH"`    → ApnPushStrip    (48px — STARTUP / PUSH BACK / DE-ICE)
 *  - `"ARR"`     → ApnArrStrip     (48px yellow — TWY ARR / STAND)
 *  - `"TAXI-DEP"` → ApnTaxiDepStrip (APN-TAXI-DEP bays)
 */
export function FlightStrip({ status, pdcStatus, ...rest }: FlightStripProps) {
  const props: StripProps = {
    ...rest,
    pdcStatus: pdcStatus ?? ("NONE" as PdcStatus),
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
    case "CLX-HALF":
      return <ClxHalfStrip {...props} />;
    case "TAXI-DEP":
      return <ApnTaxiDepStrip {...props} />;
    default:
      return null;
  }
}
