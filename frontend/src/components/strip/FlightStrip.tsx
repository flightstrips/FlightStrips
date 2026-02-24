import type { PdcStatus } from "@/api/models";
import { DelStrip } from "./DelStrip";
import { GroundStrip } from "./GroundStrip";
import { HalfStrip } from "./HalfStrip";
import type { StripProps, StripStatus } from "./types";

export type { StripStatus };
export type { StripProps };

export interface FlightStripProps extends StripProps {
  status?: StripStatus;
}

/**
 * FlightStrip – top-level strip dispatcher.
 *
 * Renders the correct strip variant based on `status`:
 *  - `"CLR"`   → DelStrip   (pre-clearance)
 *  - `"CLROK"` → GroundStrip (post-clearance, ground movement)
 *  - `"HALF"`  → HalfStrip  (compact pushback/taxi bay)
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
      return <GroundStrip {...props} />;
    case "HALF":
      return <HalfStrip {...props} />;
    default:
      return null;
  }
}
