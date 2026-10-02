import type {ReactNode} from "react";

import type {AMANCapacityReservation, AMANDataStatus, AMANFlight, AMANRunwayClosure, AMANRunwayGap, AMANTimelineMapping} from "@/api/aman";
import {cn} from "@/lib/utils";
import {AMANAxisTopPercent, AMANTimelineAxis, AMAN_TIMELINE_RULER_HALF_WIDTH, formatAMANAxisLabel} from "./AMANTimelineAxis";
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
    return null;
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
            data-testid={`operational-marker-${marker.flight.callsign}`}
            key={marker.flight.callsign}
            role="listitem"
            style={{left: rulerEdge, top: `calc(${top}% + ${offset}px)`}}
          >
            {renderTarget(marker.flight)}
          </div>
        );
      })}
    </div>
  );
}

function laneLabelPosition(index: number, side: "left" | "right"): number {
  const position = FMP_AXIS_POSITIONS[index] ?? (index + 1) / 4 * 100;
  if (side === "left") {
    const previous = index === 0 ? 0 : FMP_AXIS_POSITIONS[index - 1] ?? 0;
    return index === 0 ? position / 2 : (previous + position * 3) / 4;
  }
  const next = index === FMP_AXIS_POSITIONS.length - 1 ? 100 : FMP_AXIS_POSITIONS[index + 1] ?? 100;
  return index === FMP_AXIS_POSITIONS.length - 1 ? (position + 100) / 2 : (position * 3 + next) / 4;
}

export function FMPPairedTimelineFooter({mappings, clockMs, range, onOpenTargetInformation}: {
  mappings: AMANTimelineMapping[];
  clockMs: number;
  range: AMANTimelineRange;
  onOpenTargetInformation?: () => void;
}) {
  return (
    <footer aria-label="Feeder lane footer" className="absolute bottom-0 left-6 right-0 z-40 h-11 border-t border-[#202020] bg-[#202020] font-display text-[#dcdcdc]" data-testid="aman-timeline-footer">
      <div className="relative h-5 bg-[#555355] text-[9px] font-semibold">
        {mappings.flatMap((mapping, index) => (["left", "right"] as const).map((side) => {
          const family = mapping[side];
          return <span
            aria-label={family === null ? `Unused ${side} side` : `${family} feeder lane`}
            className={cn("absolute top-1 -translate-x-1/2 whitespace-nowrap", family === null && "font-normal uppercase text-slate-400")}
            key={`${mapping.id}-${side}`}
            style={{left: `${laneLabelPosition(index, side)}%`}}
          >{family ?? "Unused"}</span>;
        }))}
        {mappings.map((mapping, index) => {
          const position = FMP_AXIS_POSITIONS[index] ?? (index + 1) / (mappings.length + 1) * 100;
          return <button
            aria-haspopup={onOpenTargetInformation ? "dialog" : undefined}
            aria-label={onOpenTargetInformation ? "Open target information preferences" : undefined}
            className="absolute top-1/2 z-10 -translate-x-1/2 -translate-y-1/2 rounded border border-[#dcdcdc] bg-[#555355] px-1.5 py-0.5 text-[9px] leading-none hover:bg-[#a3d5e8] hover:text-white focus-visible:outline focus-visible:outline-2 focus-visible:outline-white"
            disabled={!onOpenTargetInformation}
            key={mapping.id}
            onClick={onOpenTargetInformation}
            style={{left: `${position}%`}}
            type="button"
          >{formatAMANAxisLabel(clockMs, range.startMs)}</button>;
        })}
      </div>
      {mappings.flatMap((mapping, index) => (["left", "right"] as const).map((side) => mapping[side] === null ? null : (
        <span aria-hidden="true" className="absolute top-5 -translate-x-1/2 text-[13px] leading-none text-[#d59400]" key={`${mapping.id}-${side}-cue`} style={{left: `${laneLabelPosition(index, side)}%`}}>⌄</span>
      )))}
    </footer>
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
  gapRemovalDisabled = false,
  onRemoveGap,
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
  gapRemovalDisabled?: boolean;
  onRemoveGap?: (gap: AMANRunwayGap, runway: string) => void;
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
              <AMANTimelineAxis clockMs={clockMs} range={range} showFooterButton={false} status={status} />
              <RunwayGapOverlay disabled={gapRemovalDisabled} gaps={gaps} onRemove={onRemoveGap} range={range} runway={runway} />
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
