import type {ReactNode} from "react";

import type {AMANDataStatus, AMANFlight, AMANRunwayGroup} from "@/api/aman";
import {AMANAxisTopPercent, AMANTimelineAxis} from "./AMANTimelineAxis";
import {layoutTimelineMarkers, type AMANTimelineRange} from "./presentation";
import {RunwayGapOverlay} from "./RunwayGapOverlay";

const RULER_HALF_WIDTH = 29;
const TARGET_TRACK_HEIGHT = 30;

export function ACCTimeline({flights, runwayGroups = [], range, clockMs, currentPosition, status, renderTarget}: {
  flights: AMANFlight[];
  runwayGroups?: AMANRunwayGroup[];
  range: AMANTimelineRange;
  clockMs: number;
  currentPosition: number | null;
  status: AMANDataStatus;
  renderTarget: (flight: AMANFlight) => ReactNode;
}) {
  const gap = 60_000 / (range.endMs - range.startMs) * 100;
  const markers = layoutTimelineMarkers(flights, range, gap);

  return (
    <section aria-label="ACC authoritative arrival sequence" className="relative h-full min-w-[520px] flex-1" data-testid="acc-timeline">
      <div className="absolute inset-x-0 bottom-0 top-5">
        {currentPosition !== null && <div aria-hidden="true" className="pointer-events-none absolute inset-x-0 bottom-0 bg-[#464646]" style={{top: `${currentPosition}%`}} />}
        <AMANTimelineAxis clockMs={clockMs} range={range} status={status} />
        {runwayGroups.map((group) => <RunwayGapOverlay gaps={group.gaps ?? []} key={group.id} range={range} runway={group.id} />)}
        <div aria-label="ACC arrivals in authoritative order" role="list">
          {markers.map((marker) => {
            const top = AMANAxisTopPercent(marker.timestamp, range) ?? 0;
            const offset = -marker.track * TARGET_TRACK_HEIGHT;
            return (
              <div
                aria-label={`${marker.flight.callsign} at ${marker.timestamp}`}
                className="absolute z-20 flex min-h-7 -translate-x-full -translate-y-1/2 items-center"
                data-marker-time={marker.timestamp}
                data-sequence={marker.flight.order ?? marker.flight.slot?.sequence ?? undefined}
                key={marker.flight.flight_id}
                role="listitem"
                style={{left: `calc(50% - ${RULER_HALF_WIDTH}px)`, top: `calc(${top}% + ${offset}px)`}}
              >
                {renderTarget(marker.flight)}
                <span aria-hidden="true" className="h-px w-6 shrink-0 bg-[#a9bdc5]" />
              </div>
            );
          })}
        </div>
        <span className="absolute bottom-3 left-3 font-display text-sm font-bold text-white">ACC</span>
      </div>
    </section>
  );
}
