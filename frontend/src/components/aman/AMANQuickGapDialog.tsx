import {useEffect, useState} from "react";

import type {AMANCommandIntent, AMANFlight, AMANRunwayGroup} from "@/api/aman";
import {Dialog, DialogContent, DialogTitle} from "@/components/ui/dialog";
import {cn} from "@/lib/utils";

type GapPlacement = "before" | "after";

function quickGapIntent(flight: AMANFlight, runwayGroupID: string, placement: GapPlacement, minutes: number): AMANCommandIntent | null {
  const anchor = Date.parse(flight.slot?.time ?? "");
  if (!Number.isFinite(anchor) || !Number.isInteger(minutes) || minutes < 1 || !runwayGroupID) return null;
  const duration = minutes * 60_000;
  const start = placement === "before" ? anchor - duration : anchor;
  const end = placement === "before" ? anchor : anchor + duration;
  return {
    type: "aman.create_gap",
    runway_group_id: runwayGroupID,
    start: new Date(start).toISOString(),
    end: new Date(end).toISOString(),
    label: `${minutes} min ${placement} ${flight.callsign}`,
  };
}

export function AMANQuickGapDialog({flight, groups, open, disabled, busy, rejection, onOpenChange, onCommand}: {
  flight: AMANFlight | null;
  groups: AMANRunwayGroup[];
  open: boolean;
  disabled: boolean;
  busy: boolean;
  rejection?: string | null;
  onOpenChange: (open: boolean) => void;
  onCommand: (intent: AMANCommandIntent) => void;
}) {
  const [placement, setPlacement] = useState<GapPlacement>("after");
  const [minutes, setMinutes] = useState("5");
  const [runway, setRunway] = useState("");
  const selectedRunway = groups.some((group) => group.id === runway)
    ? runway
    : groups.some((group) => group.id === flight?.runway_group_id)
      ? flight!.runway_group_id!
      : groups[0]?.id ?? "";
  const intent = flight === null ? null : quickGapIntent(flight, selectedRunway, placement, Number(minutes));

  useEffect(() => {
    if (!open) return;
    setPlacement("after");
    setMinutes("5");
    setRunway(flight?.runway_group_id ?? groups[0]?.id ?? "");
  }, [flight, groups, open]);

  return <Dialog onOpenChange={onOpenChange} open={open}>
    <DialogContent className="w-[min(25rem,calc(100vw-1rem))] max-w-none gap-0 rounded-md border-2 border-[#dcdcdc] bg-[#5174b8] p-2 font-display text-[11px] font-bold text-white [&>button]:hidden">
      <DialogTitle className="sr-only">Insert GAP {flight?.callsign ?? ""}</DialogTitle>
      <div className="grid grid-cols-[5.5rem_5.5rem_4rem_minmax(5rem,1fr)] items-start gap-1">
        <div className="grid gap-1">
          {(["before", "after"] as const).map((value) => <button aria-pressed={placement === value} className={cn("min-h-8 rounded border border-[#dcdcdc] px-2 capitalize", placement === value ? "bg-[#a3d5e8] text-[#10265c]" : "bg-[#5174b8]")} key={value} onClick={() => setPlacement(value)} type="button">{value}</button>)}
        </div>
        <div className="grid gap-1">
          <span className="grid min-h-8 place-items-center rounded border border-[#dcdcdc] bg-[#5174b8] text-[#d7e4ff]">Tag</span>
          <span className="grid min-h-8 place-items-center truncate rounded border border-[#dcdcdc] bg-[#5174b8] px-1" title={flight?.callsign}>{flight?.callsign ?? "Unavailable"}</span>
        </div>
        <div className="grid grid-cols-[1fr_2rem] items-center gap-1">
          <label className="text-center" htmlFor="quick-gap-minutes">Min</label>
          <input autoFocus aria-label="GAP minutes" className="h-7 min-w-0 border border-[#dcdcdc] bg-white text-center text-black" id="quick-gap-minutes" min="1" onChange={(event) => setMinutes(event.target.value)} type="number" value={minutes} />
          <button className="col-span-2 min-h-7 rounded border border-[#dcdcdc] bg-[#86a4af] text-white disabled:opacity-40" disabled={disabled || busy || intent === null} onClick={() => {
            if (intent === null) return;
            onCommand(intent);
            onOpenChange(false);
          }} type="button">OK</button>
        </div>
        <div aria-label="GAP runway group" className="grid gap-1" role="group">
          {groups.map((group) => <button aria-pressed={selectedRunway === group.id} className={cn("min-h-7 rounded border border-[#dcdcdc] px-1", selectedRunway === group.id ? "bg-[#a3d5e8] text-[#10265c]" : "bg-[#5174b8]")} key={group.id} onClick={() => setRunway(group.id)} type="button">{group.id.replace(/^ARRIVAL-/, "")}</button>)}
        </div>
      </div>
      {intent === null && <p className="mt-2 text-center text-amber-100" role="status">A sequenced aircraft, runway, and whole-minute duration are required.</p>}
      {busy && <p className="mt-2 text-center text-sky-100" role="status">GAP change pending</p>}
      {rejection && <p className="mt-2 border border-red-200 bg-red-950 p-1 text-center" role="alert">{rejection}</p>}
    </DialogContent>
  </Dialog>;
}
