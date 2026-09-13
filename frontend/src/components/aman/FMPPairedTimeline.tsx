import type {ReactNode} from "react";

import type {AMANCapacityReservation, AMANDataStatus, AMANFlight, AMANRunwayClosure, AMANRunwayGap, AMANTimelineMapping} from "@/api/aman";
import {cn} from "@/lib/utils";
import {AMANAxisTopPercent, AMANTimelineAxis} from "./AMANTimelineAxis";
import {layoutTimelineMarkers, type AMANTimelineRange} from "./presentation";
import {RunwayGapOverlay} from "./RunwayGapOverlay";
import {RunwayClosureOverlay} from "./RunwayClosureOverlay";
import {CapacityReservationOverlay} from "./CapacityReservationOverlay";

const RULER_HALF_WIDTH = 29;
const TARGET_TRACK_HEIGHT = 30;
// Figma node 3056:371 places the three ruler centres at 264.91, 571.91,
// and 1045.91 on the 1280 px MAESTRO canvas. Keeping those proportions
// preserves the shared TUDLO/MONAK and TIDVU/ERNOV target fields.
const FMP_AXIS_POSITIONS = [20.696, 44.681, 81.712] as const;

function familyOf(flight: AMANFlight): string | null {
  return flight.star_family ?? flight.feeder ?? flight.star;
}

function FeederSide({
  family,
  flights,
  range,
  side,
  renderTarget,
}: {
  family: string | null;
  flights: AMANFlight[];
  range: AMANTimelineRange;
  side: "left" | "right";
  renderTarget: (flight: AMANFlight) => ReactNode;
}) {
  if (family === null) {
    return <div aria-label={`Unused ${side} side`} className={cn("absolute bottom-3 text-[11px] uppercase text-slate-400", side === "left" ? "left-3" : "right-3")}>Unused</div>;
  }

  const familyFlights = flights.filter((flight) => familyOf(flight) === family);
  const gap = 60_000 / (range.endMs - range.startMs) * 100;
  const markers = layoutTimelineMarkers(familyFlights, range, gap);
  const rulerEdge = side === "left" ? `calc(50% - ${RULER_HALF_WIDTH}px)` : `calc(50% + ${RULER_HALF_WIDTH}px)`;

  return (
    <div aria-label={`${family} arrivals`} role="list">
      {markers.map((marker) => {
        const top = AMANAxisTopPercent(marker.timestamp, range) ?? 0;
        const offset = -marker.track * TARGET_TRACK_HEIGHT;
        return (
          <div
            aria-label={`${marker.flight.callsign} at ${marker.timestamp}`}
            className={cn("absolute z-20 flex min-h-7 -translate-y-1/2 items-center", side === "left" && "-translate-x-full")}
            data-family={family}
            data-marker-time={marker.timestamp}
            data-track={marker.track}
            data-testid={`operational-marker-${marker.flight.flight_id}`}
            key={marker.flight.flight_id}
            role="listitem"
            style={{left: rulerEdge, top: `calc(${top}% + ${offset}px)`}}
          >
            {renderTarget(marker.flight)}
          </div>
        );
      })}
      <span className={cn("absolute bottom-3 whitespace-nowrap font-display text-sm font-bold text-white", side === "left" ? "right-[35px]" : "left-[35px]")}>{family}</span>
    </div>
  );
}

export function FMPPairedTimeline({
  mappings,
  flights,
  range,
  clockMs,
  currentPosition,
  status,
  renderTarget,
  gaps = [],
  closures = [],
  capacityReservations = [],
  runway = "runway",
}: {
  mappings: AMANTimelineMapping[];
  flights: AMANFlight[];
  range: AMANTimelineRange;
  clockMs: number;
  currentPosition: number | null;
  status: AMANDataStatus;
  renderTarget: (flight: AMANFlight) => ReactNode;
  gaps?: AMANRunwayGap[];
  closures?: AMANRunwayClosure[];
  capacityReservations?: AMANCapacityReservation[];
  runway?: string;
}) {
  return (
    <div className="relative h-full min-w-[50rem] flex-1" data-testid="fmp-paired-layout">
      {currentPosition !== null && (
        <div aria-hidden="true" className="pointer-events-none absolute inset-x-0 bottom-0 top-5">
          <div className="absolute inset-x-0 bottom-0 bg-[#464646]" style={{top: `${currentPosition}%`}} />
        </div>
      )}
      {mappings.map((mapping, index) => {
        const position = FMP_AXIS_POSITIONS[index] ?? (index + 1) / (mappings.length + 1) * 100;
        return (
          <section
            aria-label={`Timeline ${mapping.id}: ${mapping.left ?? "unused"} left, ${mapping.right ?? "unused"} right`}
            className="absolute bottom-0 top-5 -translate-x-1/2"
            data-testid={`fmp-timeline-${mapping.id}`}
            key={mapping.id}
            style={{left: `${position}%`, width: "max(6%, 58px)"}}
          >
            <div className="absolute inset-0">
              <AMANTimelineAxis clockMs={clockMs} range={range} status={status} />
              <RunwayGapOverlay gaps={gaps} range={range} runway={runway} />
              <RunwayClosureOverlay closures={closures} range={range} runway={runway} status={status} />
              <CapacityReservationOverlay range={range} reservations={capacityReservations} runway={runway} status={status} />
              <FeederSide family={mapping.left} flights={flights} range={range} renderTarget={renderTarget} side="left" />
              <FeederSide family={mapping.right} flights={flights} range={range} renderTarget={renderTarget} side="right" />
            </div>
          </section>
        );
      })}
    </div>
  );
}
