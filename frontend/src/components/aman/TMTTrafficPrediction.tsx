import {useState} from "react";

import type {AMANTrafficBucket, AMANTrafficPrediction} from "@/api/aman";
import {cn} from "@/lib/utils";

function timeLabel(timestamp: string): string {
  return timestamp.slice(11, 16);
}

function alertTone(bucket: AMANTrafficBucket): string {
  if (bucket.alert === "red") return "border-[#9c0000] bg-[#9c0000]";
  if (bucket.alert === "yellow") return "border-[#f0e129] bg-[#f0e129]";
  return "border-[#777] bg-[#343434]";
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
    <section aria-label="TMT traffic prediction" className="flex min-h-0 flex-col border border-[#777] bg-[#292929] text-white">
      <header className="flex items-center gap-2 border-b border-[#777] px-3 py-2">
        <h2 className="font-display text-sm font-bold tracking-wide">TMT · TRAFFIC PREDICTION</h2>
        <span className="text-[10px] uppercase text-slate-300">VATSIM {prediction.source_status}</span>
        <span className={cn("ml-auto rounded border px-1.5 py-0.5 text-[10px] font-bold uppercase", prediction.status === "ready" ? "border-emerald-400 text-emerald-200" : "border-amber-300 text-amber-200")}>{prediction.status}</span>
      </header>

      {prediction.degraded_reasons.length > 0 && (
        <div className="border-b border-amber-400/60 bg-amber-950/70 px-3 py-2 text-xs text-amber-100" role="status">
          {prediction.degraded_reasons.map(reasonLabel).join(" ")}
        </div>
      )}

      <div className="grid min-h-[260px] flex-1 grid-cols-12 gap-px overflow-x-auto bg-[#777]" role="list">
        {prediction.buckets.map((bucket) => {
          const details = bucketDetails(bucket);
          return (
            <article
              aria-label={`${timeLabel(bucket.start)} to ${timeLabel(bucket.end)}: ${bucket.count} arrivals, load factor ${bucket.load_factor}`}
              className={cn("group relative flex min-w-0 flex-col justify-end border-t-4 px-0.5 pb-2 pt-6 text-center text-black", alertTone(bucket))}
              key={bucket.start}
              onBlur={() => setActiveBucketStart(null)}
              onFocus={() => setActiveBucketStart(bucket.start)}
              onMouseEnter={() => setActiveBucketStart(bucket.start)}
              onMouseLeave={() => setActiveBucketStart(null)}
              role="listitem"
              tabIndex={0}
              title={details}
            >
              <div className="absolute inset-x-0 top-1 truncate px-px text-[9px] font-bold text-white drop-shadow">{bucket.selected_rate === null ? "RATE —" : `${bucket.selected_rate.runway_group_id} · ${bucket.selected_rate.arrivals_per_hour}`}</div>
              <div className="mx-auto flex h-[180px] w-5 flex-col justify-end overflow-hidden border border-black bg-[#222] sm:w-7" aria-hidden="true">
                {bucket.airborne_count > 0 && <div className="w-full bg-[#dcdcdc]" style={{height: `${Math.max(8, bucket.airborne_count * 8)}px`}} />}
                {bucket.planned_count > 0 && <div className="w-full bg-[#96d796]" style={{height: `${Math.max(8, bucket.planned_count * 8)}px`}} />}
              </div>
              <strong className="mt-1 font-mono text-sm text-white">{bucket.load_factor}</strong>
              <span className="font-mono text-[10px] text-white">{timeLabel(bucket.start)}</span>
            </article>
          );
        })}
      </div>

      <div aria-live="polite" className="min-h-14 border-t border-[#777] bg-black px-3 py-2 text-xs text-white">
        {activeBucket === null
          ? "Hover or focus a quarter-hour bucket for flight details."
          : <><strong className="mr-2 font-mono">{timeLabel(activeBucket.start)}–{timeLabel(activeBucket.end)}</strong><span className="whitespace-pre-line">{bucketDetails(activeBucket)}</span></>}
      </div>

      <footer className="flex flex-wrap items-center gap-x-3 gap-y-1 border-t border-[#777] px-3 py-2 text-[10px] text-slate-200">
        <span><i className="mr-1 inline-block h-2.5 w-2.5 bg-[#96d796]" />Planned</span>
        <span><i className="mr-1 inline-block h-2.5 w-2.5 bg-[#dcdcdc]" />Airborne</span>
        <span>Load = aircraft × 4</span>
        <span className="ml-auto font-mono">{timeLabel(prediction.range_start)}–{timeLabel(prediction.range_end)} UTC</span>
      </footer>
    </section>
  );
}
