import type {ReactNode} from "react";

import type {AMANDataStatus, AMANFlight, AMANRunwayClosure, AMANRunwayGap, AMANTimelineMapping} from "@/api/aman";
import {cn} from "@/lib/utils";
import {AMANAxisTopPercent, AMANTimelineAxis} from "./AMANTimelineAxis";
import {layoutTimelineMarkers, type AMANTimelineRange} from "./presentation";
import {RunwayGapOverlay} from "./RunwayGapOverlay";
import {RunwayClosureOverlay} from "./RunwayClosureOverlay";

const RULER_HALF_WIDTH = 29;
const TARGET_TRACK_HEIGHT = 30;

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
            key={marker.flight.flight_id}
            role="listitem"
            style={{left: rulerEdge, top: `calc(${top}% + ${offset}px)`}}
          >
            {side === "right" && <span aria-hidden="true" className="h-px w-6 shrink-0 bg-[#a9bdc5]" />}
            {renderTarget(marker.flight)}
            {side === "left" && <span aria-hidden="true" className="h-px w-6 shrink-0 bg-[#a9bdc5]" />}
          </div>
        );
      })}
      <span className={cn("absolute bottom-3 font-display text-sm font-bold text-white", side === "left" ? "left-3" : "right-3")}>{family}</span>
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
  runway?: string;
}) {
  return (
    <>
      {mappings.map((mapping) => (
        <section
          aria-label={`Timeline ${mapping.id}: ${mapping.left ?? "unused"} left, ${mapping.right ?? "unused"} right`}
          className="relative h-full min-w-[520px]"
          data-testid={`fmp-timeline-${mapping.id}`}
          key={mapping.id}
        >
          <div className="absolute inset-x-0 bottom-0 top-5">
            {currentPosition !== null && <div aria-hidden="true" className="pointer-events-none absolute inset-x-0 bottom-0 bg-[#464646]" style={{top: `${currentPosition}%`}} />}
            <AMANTimelineAxis clockMs={clockMs} range={range} status={status} />
            <RunwayGapOverlay gaps={gaps} range={range} runway={runway} />
            <RunwayClosureOverlay closures={closures} range={range} runway={runway} status={status} />
            <FeederSide family={mapping.left} flights={flights} range={range} renderTarget={renderTarget} side="left" />
            <FeederSide family={mapping.right} flights={flights} range={range} renderTarget={renderTarget} side="right" />
          </div>
        </section>
      ))}
    </>
  );
}
