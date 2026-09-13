import type {ReactNode} from "react";

import type {AMANCapacityReservation, AMANDataStatus, AMANFlight, AMANRunwayClosure, AMANRunwayGap, AMANTimelineMapping} from "@/api/aman";
import {cn} from "@/lib/utils";
import {AMANAxisTopPercent, AMANTimelineAxis, AMAN_TIMELINE_RULER_HALF_WIDTH} from "./AMANTimelineAxis";
import {layoutTimelineMarkers, type AMANTimelineRange} from "./presentation";
import {RunwayGapOverlay} from "./RunwayGapOverlay";
import {RunwayClosureOverlay} from "./RunwayClosureOverlay";
import {CapacityReservationOverlay} from "./CapacityReservationOverlay";

const TARGET_TRACK_HEIGHT = 30;
// The current AMAN layout uses three equal ruler columns. Twenty-percent side
// gutters leave a full compact-target track outside each edge ruler, while the
// 30-percent gaps fit the two inward-facing STAR tracks side by side.
const FMP_AXIS_POSITIONS = [20, 50, 80] as const;

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
    return <div aria-label={`Unused ${side} side`} className={cn("absolute -bottom-9 whitespace-nowrap text-[11px] uppercase text-slate-400", side === "left" ? "right-[29px]" : "left-[29px]")}>Unused</div>;
  }

  const familyFlights = flights.filter((flight) => familyOf(flight) === family);
  const gap = 60_000 / (range.endMs - range.startMs) * 100;
  const markers = layoutTimelineMarkers(familyFlights, range, gap);
  const rulerEdge = side === "left" ? `calc(50% - ${AMAN_TIMELINE_RULER_HALF_WIDTH}px)` : `calc(50% + ${AMAN_TIMELINE_RULER_HALF_WIDTH}px)`;

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
      <span className={cn("absolute -bottom-9 whitespace-nowrap font-display text-xs font-semibold text-[#dcdcdc]", side === "left" ? "right-[29px]" : "left-[29px]")}>{family}</span>
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
  const finalTenBoundary = Math.max(0, (currentPosition ?? 100) - 10 * 60_000 / (range.endMs - range.startMs) * 100);
  return (
    <div className="relative h-full min-w-0 flex-1" data-current-position={currentPosition ?? undefined} data-testid="fmp-paired-layout">
      <div aria-hidden="true" className="pointer-events-none absolute inset-x-0 bottom-11 top-5">
        <div className="absolute inset-x-0 bottom-0 border-t-2 border-[#9c0000] bg-[#3f3f3f]" style={{top: `${finalTenBoundary}%`}} />
      </div>
      {mappings.map((mapping, index) => {
        const position = FMP_AXIS_POSITIONS[index] ?? (index + 1) / (mappings.length + 1) * 100;
        return (
          <section
            aria-label={`Timeline ${mapping.id}: ${mapping.left ?? "unused"} left, ${mapping.right ?? "unused"} right`}
            className="absolute bottom-11 top-5 -translate-x-1/2"
            data-testid={`fmp-timeline-${mapping.id}`}
            key={mapping.id}
            style={{left: `${position}%`, width: "max(6%, 46px)"}}
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
