import {useState} from "react";

import type {AMANTrafficBucket, AMANTrafficPrediction} from "@/api/aman";

const CHART_MIN = 16;
const CHART_MAX = 50;
const TICKS = Array.from({length: 17}, (_, index) => CHART_MAX - index * 2);

function timeLabel(timestamp: string): string {
  return timestamp.slice(11, 16);
}

function chartHeight(value: number): number {
  if (value <= 0) return 0;
  return Math.max(2, Math.min(100, ((value - CHART_MIN) / (CHART_MAX - CHART_MIN)) * 100));
}

function alertColour(bucket: AMANTrafficBucket): string | null {
  if (bucket.alert === "red") return "#9c0000";
  if (bucket.alert === "yellow") return "#f0e129";
  return null;
}

function reasonLabel(reason: string): string {
  if (reason === "missing_selected_rate") return "Arrival rate unavailable; overload status is not estimated.";
  if (reason === "stale_flight_data") return "Some flight timing data is stale.";
  if (reason === "vatsim_disconnected") return "VATSIM timing source is disconnected.";
  if (reason.startsWith("missing_timing:")) return `No usable timing for ${reason.slice("missing_timing:".length)}.`;
  return reason.replace(/_/g, " ");
}

function bucketDetails(bucket: AMANTrafficBucket): string {
  return bucket.flights.length === 0
    ? "No predicted arrivals"
    : bucket.flights.map((flight) => `${flight.callsign} ${timeLabel(flight.landing_at)} · ${flight.timing_source.replace("vatsim_", "VATSIM ")} · ${flight.data_status}`).join("\n");
}

export function TMTTrafficPrediction({prediction}: {prediction: AMANTrafficPrediction}) {
  const [activeBucketStart, setActiveBucketStart] = useState<string | null>(null);
  const activeBucket = prediction.buckets.find((bucket) => bucket.start === activeBucketStart) ?? null;

  return (
    <section aria-label="TMT traffic prediction" className="relative flex min-h-0 flex-col overflow-hidden border border-[#777] bg-[#3c3c3c] text-white">
      {prediction.status !== "ready" && (
        <div className="absolute right-1 top-1 z-20 max-w-[70%] border border-amber-300 bg-[#40200f] px-2 py-1 text-[10px] text-amber-100" role="status">
          <strong className="mr-1 uppercase">{prediction.status}</strong>
          {prediction.degraded_reasons.map(reasonLabel).join(" ")}
        </div>
      )}

      <div className="flex min-h-0 flex-1">
        <div aria-hidden="true" className="relative mb-6 mt-2 w-9 shrink-0 font-mono text-[10px] font-bold text-[#dcdcdc]">
          {TICKS.map((tick) => (
            <span className="absolute right-1 -translate-y-1/2" key={tick} style={{top: `${((CHART_MAX - tick) / (CHART_MAX - CHART_MIN)) * 100}%`}}>{tick}</span>
          ))}
        </div>

        <div className="relative min-w-0 flex-1">
          <div aria-hidden="true" className="pointer-events-none absolute inset-x-0 bottom-6 top-2 z-0">
            {TICKS.map((tick) => (
              <i className="absolute inset-x-0 border-t border-[#777]" key={tick} style={{top: `${((CHART_MAX - tick) / (CHART_MAX - CHART_MIN)) * 100}%`}} />
            ))}
          </div>

          <div className="relative z-10 grid h-full" role="list" style={{gridTemplateColumns: `repeat(${prediction.buckets.length}, minmax(2.75rem, 1fr))`}}>
            {prediction.buckets.map((bucket) => {
              const totalHeight = chartHeight(bucket.load_factor);
              const plannedShare = bucket.count === 0 ? 0 : bucket.planned_count / bucket.count;
              const selectedRate = bucket.selected_rate?.arrivals_per_hour ?? null;
              const overload = selectedRate === null ? 0 : Math.max(0, bucket.load_factor - selectedRate);
              const alertShare = bucket.alert === "none" || bucket.load_factor === 0
                ? 0
                : Math.min(1, (overload > 0 ? overload : 4) / bucket.load_factor);
              const details = bucketDetails(bucket);
              return (
                <article
                  aria-label={`${timeLabel(bucket.start)} to ${timeLabel(bucket.end)}: ${bucket.count} arrivals, load factor ${bucket.load_factor}`}
                  className="group flex min-w-0 flex-col border-l border-black/70 first:border-l-0 focus-visible:z-20 focus-visible:outline focus-visible:outline-2 focus-visible:outline-white"
                  key={bucket.start}
                  onBlur={() => setActiveBucketStart(null)}
                  onFocus={() => setActiveBucketStart(bucket.start)}
                  onMouseEnter={() => setActiveBucketStart(bucket.start)}
                  onMouseLeave={() => setActiveBucketStart(null)}
                  role="listitem"
                  tabIndex={0}
                  title={details}
                >
                  <div className="relative mt-2 min-h-0 flex-1" aria-hidden="true">
                    {bucket.load_factor > 0 && (
                      <div className="absolute inset-x-0 bottom-0 flex flex-col-reverse overflow-hidden border border-black bg-[#dcdcdc]" data-testid={`traffic-bar-${timeLabel(bucket.start)}`} style={{height: `${totalHeight}%`}}>
                        {bucket.planned_count > 0 && <div className="w-full shrink-0 bg-[#96d796]" style={{height: `${plannedShare * 100}%`}} />}
                        {bucket.airborne_count > 0 && <div className="min-h-0 flex-1 bg-[#dcdcdc]" />}
                        {alertColour(bucket) !== null && alertShare > 0 && (
                          <div className="absolute inset-x-0 top-0" data-alert={bucket.alert} style={{backgroundColor: alertColour(bucket) ?? undefined, height: `${alertShare * 100}%`}} />
                        )}
                      </div>
                    )}
                  </div>
                  <time className="grid h-6 shrink-0 place-items-center border-y border-[#dcdcdc] bg-[#454545] font-mono text-[10px] font-bold text-white" dateTime={bucket.start}>{timeLabel(bucket.start)}</time>
                </article>
              );
            })}
          </div>

          {activeBucket !== null && (
            <div className="pointer-events-none absolute bottom-7 left-1 z-30 max-h-24 max-w-[calc(100%_-_0.5rem)] overflow-hidden border border-[#dcdcdc] bg-black/95 px-2 py-1 text-[10px] leading-4 text-white shadow-lg" aria-hidden="true">
              <strong className="mr-2 font-mono">{timeLabel(activeBucket.start)}–{timeLabel(activeBucket.end)}</strong>
              <span className="whitespace-pre-line">{bucketDetails(activeBucket)}</span>
            </div>
          )}
        </div>
      </div>

      <p className="sr-only" aria-live="polite">
        {activeBucket === null ? "Focus a quarter-hour bucket for flight details." : `${timeLabel(activeBucket.start)} to ${timeLabel(activeBucket.end)}. ${bucketDetails(activeBucket)}`}
      </p>
      <p className="sr-only">Planned traffic is green. Airborne traffic is light grey. Load factor equals aircraft count times four. Range {timeLabel(prediction.range_start)} to {timeLabel(prediction.range_end)} UTC. Source {prediction.source_status}.</p>
    </section>
  );
}
