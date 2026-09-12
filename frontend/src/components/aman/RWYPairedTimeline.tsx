import type {ReactNode} from "react";

import type {AMANDataStatus, AMANFlight, AMANState} from "@/api/aman";
import {cn} from "@/lib/utils";
import {AMANAxisTopPercent, AMANTimelineAxis} from "./AMANTimelineAxis";
import {buildRWYTimelineLanes, layoutTimelineMarkers, type AMANFlightLane, type AMANTimelineRange} from "./presentation";
import {RunwayGapOverlay} from "./RunwayGapOverlay";
import {RunwayClosureOverlay} from "./RunwayClosureOverlay";

const RULER_HALF_WIDTH = 29;
const TARGET_TRACK_HEIGHT = 30;

function RunwaySide({lane, range, side, placement, renderTarget, status}: {
  lane: AMANFlightLane | undefined;
  range: AMANTimelineRange;
  side: "left" | "right";
  placement: string;
  renderTarget: (flight: AMANFlight) => ReactNode;
  status: AMANDataStatus;
}) {
  if (lane === undefined) return <span aria-hidden="true" className={cn("absolute bottom-3 text-[11px] uppercase text-slate-500", side === "left" ? "left-3" : "right-3")}>No active runway</span>;

  const gap = 60_000 / (range.endMs - range.startMs) * 100;
  const markers = layoutTimelineMarkers(lane.flights, range, gap);
  const rulerEdge = side === "left" ? `calc(50% - ${RULER_HALF_WIDTH}px)` : `calc(50% + ${RULER_HALF_WIDTH}px)`;
  return (
    <div aria-label={`${lane.id} active runway arrivals`} data-placement={placement} data-testid={`rwy-lane-${lane.id}`} role="list">
      <RunwayGapOverlay gaps={lane.gaps ?? []} range={range} runway={lane.id} />
      <RunwayClosureOverlay closures={lane.closures ?? []} range={range} runway={lane.id} status={status} />
      {markers.map((marker) => {
        const top = AMANAxisTopPercent(marker.timestamp, range) ?? 0;
        const offset = -marker.track * TARGET_TRACK_HEIGHT;
        return (
          <div
            aria-label={`${marker.flight.callsign} at ${marker.timestamp}`}
            className={cn("absolute z-20 flex min-h-7 -translate-y-1/2 items-center", side === "left" && "-translate-x-full")}
            data-marker-time={marker.timestamp}
            data-sequence={marker.flight.order ?? marker.flight.slot?.sequence ?? undefined}
            data-testid={`operational-marker-${marker.flight.flight_id}`}
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
      <span className={cn("absolute bottom-3 font-display text-sm font-bold text-white", side === "left" ? "left-3" : "right-3")}>{lane.id}</span>
    </div>
  );
}

export function RWYPairedTimeline({state, range, clockMs, currentPosition, status, renderTarget}: {
  state: AMANState;
  range: AMANTimelineRange;
  clockMs: number;
  currentPosition: number | null;
  status: AMANDataStatus;
  renderTarget: (flight: AMANFlight) => ReactNode;
}) {
  const {lanes, unavailable, truncated} = buildRWYTimelineLanes(state);
  if (unavailable) return <div className="grid min-w-full place-items-center text-sm font-semibold text-amber-200" role="status">Active runway data unavailable</div>;

  return (
    <div className="contents" aria-label="Active runway timelines" role="group">
      {truncated && <span className="sr-only" role="status">Showing the first four active runway groups</span>}
      <div aria-hidden="true" className="relative h-full min-w-[520px]" data-testid="rwy-timeline-left-spacer" />
      {[0, 1].map((timelineIndex) => (
        <section
          aria-label={`${timelineIndex === 0 ? "Middle" : "Right"} runway timeline`}
          className="relative h-full min-w-[520px] flex-1"
          data-testid={`rwy-timeline-${timelineIndex === 0 ? "middle" : "right"}`}
          key={timelineIndex}
        >
          <div className="absolute inset-x-0 bottom-0 top-5">
            {currentPosition !== null && <div aria-hidden="true" className="pointer-events-none absolute inset-x-0 bottom-0 bg-[#464646]" style={{top: `${currentPosition}%`}} />}
            <AMANTimelineAxis clockMs={clockMs} range={range} status={status} />
            <RunwaySide lane={lanes[timelineIndex * 2]} placement={`${timelineIndex === 0 ? "middle" : "right"}-left`} range={range} renderTarget={renderTarget} side="left" status={status} />
            <RunwaySide lane={lanes[timelineIndex * 2 + 1]} placement={`${timelineIndex === 0 ? "middle" : "right"}-right`} range={range} renderTarget={renderTarget} side="right" status={status} />
          </div>
        </section>
      ))}
    </div>
  );
}
