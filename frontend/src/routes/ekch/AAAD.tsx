import { Strip } from "@/components/strip/Strip.tsx";
import { MemAidButton } from "@/components/strip/TacticalButtons.tsx";
import { MessageStrip } from "@/components/strip/MessageStrip.tsx";
import { MessageComposeDialog } from "@/components/MessageComposeDialog.tsx";
import { useMyPosition, useMessages, useWebSocketStore, useDelOnline } from "@/store/store-hooks.ts";
import {
  useClearedStrips,
  useDeIceStrips,
  useFinalStrips,
  useInboundStrips,
  useNorwegianBayStrips,
  useOtherBayStrips,
  usePushbackStrips,
  useRwyArrStrips,
  useSasBayStrips,
  useStandStrips,
  useTaxiArrStrips,
  useTaxiDepStrips,
  useTaxiDepLwrStrips,
  isFlight,
} from "@/store/airports/ekch.ts";
import type { AnyStrip, FrontendStrip, StripRef } from "@/api/models.ts";
import { Bay } from "@/api/models.ts";
import type { StripStatus } from "@/components/strip/types.ts";
import { SortableBay, DropIndicatorBay } from "@/components/bays/SortableBay.tsx";
import { ViewDndContext } from "@/components/bays/ViewDndContext.tsx";
import { StripListPopup, type SortMode } from "@/components/StripListPopup.tsx";
import { useState } from "react";
import { APN_TAXI_DEP_STRIP_WIDTH } from "@/components/strip/ApnTaxiDepStrip.tsx";
import { CLS_BTN, CLS_SCROLLBAR } from "@/components/strip/shared";

// Shared header styles
const pageWrapper    = "bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center gap-2";
const activeHeader   = "bg-[#b3b3b3] h-10 flex items-center px-2 shrink-0";
const activeLabel    = "text-[#393939] font-bold text-lg";
const lockedHeader   = "bg-[#393939] h-10 flex items-center px-2 shrink-0";
const lockedLabel    = "text-white font-bold text-lg";
const primaryHeader  = "bg-primary h-10 flex items-center px-2 shrink-0";
const primaryLabel   = "text-white font-bold text-lg";
const scrollArea     = `w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto ${CLS_SCROLLBAR}`;
const col            = "w-1/4 h-full bg-[#555355] flex flex-col";
const tabBar         = "flex shrink-0 bg-[#393939]";
const tabBtn         = "flex-1 h-8 bg-[#555355] text-white font-bold text-sm border border-[#393939] hover:bg-[#6a6a6a]";
const btn            = CLS_BTN;

export default function AAAD() {
  const myPosition  = useMyPosition();
  const messages    = useMessages();
  const [composeOpen, setComposeOpen] = useState(false);
  const [arrOpen, setArrOpen] = useState(false);

  const delOnline = useDelOnline();
  // When DEL is online, APRON is not responsible for clearances → CLR/DEL panel is inactive.
  // When DEL is offline, APRON handles clearances → CLR/DEL panel is active.
  const clrDelActive = !delOnline;

  const finalStrips   = useFinalStrips().filter(isFlight);
  const rwyArrStrips  = useRwyArrStrips().filter(isFlight);
  const standStrips   = useStandStrips().filter(isFlight);
  const twyDepUpr    = useTaxiDepStrips();
  const twyDepLwr    = useTaxiDepLwrStrips();
  const twyArrStrips  = useTaxiArrStrips();
  const startupStrips = useClearedStrips().sort((a, b) => a.sequence - b.sequence);
  const pushStrips    = usePushbackStrips().filter(isFlight);
  const deIceStrips   = useDeIceStrips().filter(isFlight);
  const otherStrips   = useOtherBayStrips().sort((a, b) => a.sequence - b.sequence);
  const sasStrips     = useSasBayStrips().sort((a, b) => a.sequence - b.sequence);
  const norStrips     = useNorwegianBayStrips().sort((a, b) => a.sequence - b.sequence);

  const inboundStrips = useInboundStrips();

  const updateOrder       = useWebSocketStore(state => state.updateOrder);
  const move              = useWebSocketStore(state => state.move);
  const moveTacticalStrip = useWebSocketStore(state => state.moveTacticalStrip);
  const assumeStrip       = useWebSocketStore(state => state.assumeStrip);

  const arrSortModes: SortMode<FrontendStrip>[] = [
    { key: "ETA",      label: "ETA",      compareFn: (a, b) => a.eldt.localeCompare(b.eldt) },
    { key: "CALLSIGN", label: "CALLSIGN", compareFn: (a, b) => a.callsign.localeCompare(b.callsign) },
    { key: "ADEP",     label: "ADEP",     compareFn: (a, b) => a.origin.localeCompare(b.origin) },
  ];

  const bayStripMap = {
    "TWY-DEP-UPR": { strips: twyDepUpr,    targetBay: Bay.Taxi },
    "TWY-DEP-LWR": { strips: twyDepLwr,    targetBay: Bay.TaxiLwr },
    "TWY-ARR":     { strips: twyArrStrips, targetBay: Bay.TwyArr },
    "STARTUP":     { strips: startupStrips, targetBay: Bay.Cleared },
    "PUSHBACK":    { strips: pushStrips,    targetBay: Bay.Push },
    "DE-ICE":      { strips: deIceStrips,   targetBay: Bay.DeIce },
  };

  const transferRules: Record<string, string[]> = {
    "TWY-DEP-UPR": ["TWY-DEP-LWR", "TWY-ARR", "STARTUP", "PUSHBACK", "DE-ICE"],
    "TWY-DEP-LWR": ["TWY-DEP-UPR", "TWY-ARR", "STARTUP", "PUSHBACK", "DE-ICE"],
    "TWY-ARR":     ["TWY-DEP-UPR", "TWY-DEP-LWR", "STARTUP", "PUSHBACK"],
    "STARTUP":     ["TWY-DEP-UPR", "TWY-DEP-LWR", "TWY-ARR", "PUSHBACK", "DE-ICE"],
    "PUSHBACK":    ["TWY-DEP-UPR", "TWY-DEP-LWR", "TWY-ARR", "STARTUP", "DE-ICE"],
    "DE-ICE":      ["TWY-DEP-UPR", "TWY-DEP-LWR", "STARTUP", "PUSHBACK"],
  };

  const statusForBay: Record<string, StripStatus> = {
    "TWY-DEP-UPR": "TAXI-DEP",
    "TWY-DEP-LWR": "TAXI-DEP",
    "TWY-ARR":  "ARR",
    "STARTUP":  "PUSH",
    "PUSHBACK": "PUSH",
    "DE-ICE":   "PUSH",
  };

  return (
    <ViewDndContext
      bayStripMap={bayStripMap}
      transferRules={transferRules}
      onReorder={(activeRef: StripRef, insertAfter: StripRef | null) => {
        if (activeRef.kind === "tactical") moveTacticalStrip(activeRef.id!, insertAfter);
        else updateOrder(activeRef.callsign!, insertAfter);
      }}
      onMove={(callsign, bay) => move(callsign, bay)}
      renderDragOverlay={(strip: AnyStrip) => {
        if (!isFlight(strip)) return <Strip strip={strip} width={APN_TAXI_DEP_STRIP_WIDTH} />;
        const bayEntry = Object.entries(bayStripMap).find(([, c]) =>
          c.strips.some(s => isFlight(s) && s.callsign === strip.callsign)
        );
        if (!bayEntry) return null;
        return <Strip strip={strip} status={statusForBay[bayEntry[0]]} myPosition={myPosition} />;
      }}
    >
    <div className={pageWrapper}>

      {/* ── Col 1: MESSAGES / FINAL (locked) / RWY ARR (locked) / STAND ── */}
      <div className={col}>

        <div className={primaryHeader + " justify-between"}>
          <span className={primaryLabel}>MESSAGES</span>
          <button className={btn} onClick={() => setComposeOpen(true)}>FREE TEXT</button>
        </div>
        <div className={`h-[15%] ${scrollArea}`}>
          {messages.map(msg => (
            <MessageStrip key={msg.id} msg={msg} />
          ))}
        </div>
        <MessageComposeDialog open={composeOpen} onClose={() => setComposeOpen(false)} />

        <div className={lockedHeader + " justify-between"}>
          <span className={lockedLabel}>FINAL</span>
          <button className={btn} onClick={() => setArrOpen(true)}>ARR</button>
        </div>
        <DropIndicatorBay bayId="FINAL" className={`h-[25%] ${scrollArea}`}>
          {finalStrips.map(s => (
            <Strip key={s.callsign} strip={s} status="HALF" halfStripVariant="LOCKED-ARR" selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className={lockedHeader}>
          <span className={lockedLabel}>RWY ARR</span>
        </div>
        <DropIndicatorBay bayId="RWY-ARR" className={`h-[30%] ${scrollArea}`}>
          {rwyArrStrips.map(s => (
            <Strip key={s.callsign} strip={s} status="ARR" selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className={activeHeader}>
          <span className={activeLabel}>STAND</span>
        </div>
        <DropIndicatorBay bayId="STAND" className={`flex-1 ${scrollArea}`}>
          {standStrips.map(s => (
            <Strip key={s.callsign} strip={s} status="ARR" myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        {arrOpen && (
          <StripListPopup
            title="ARR"
            strips={inboundStrips}
            sortModes={arrSortModes}
            onRowClick={(strip) => {
              move(strip.callsign, Bay.Final);
              assumeStrip(strip.callsign);
              setArrOpen(false);
            }}
            onDismiss={() => setArrOpen(false)}
            myPosition={myPosition}
          />
        )}
      </div>

      {/* ── Col 2: TWY DEP (UPR+LWR) / TWY ARR ── */}
      <div className={col}>

        <div className={activeHeader + " justify-between"}>
          <span className={activeLabel}>TWY DEP</span>
          <span className="flex gap-1">
            <button className={btn}>NEW</button>
            <MemAidButton bay={Bay.Taxi} className={btn} />
          </span>
        </div>
        {/* TWY DEP-UPR (intermediate hold short, TAXI bay) */}
        <SortableBay
          strips={twyDepUpr}
          bayId="TWY-DEP-UPR"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[30%] ${scrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="TAXI-DEP" myPosition={myPosition} width={APN_TAXI_DEP_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>

        {/* TW / TE / GW / GE bay selector tabs */}
        <div className={tabBar}>
          {["TW", "TE", "GW", "GE"].map(tab => (
            <button
              key={tab}
              className={tabBtn}
            >
              {tab}
            </button>
          ))}
        </div>

        {/* TWY DEP-LWR (final hold short, TAXI_LWR bay) — no header */}
        <SortableBay
          strips={twyDepLwr}
          bayId="TWY-DEP-LWR"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[30%] ${scrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="TAXI-DEP" myPosition={myPosition} width={APN_TAXI_DEP_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>

        <div className={activeHeader}>
          <span className={activeLabel}>TWY ARR</span>
          <span className="ml-auto">
            <MemAidButton bay={Bay.Taxi} className={btn} />
          </span>
        </div>
        <div className={`flex-1 ${scrollArea}`}>
          <SortableBay
            strips={twyArrStrips}
            bayId="TWY-ARR"
            isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
            standalone={false}
          >
            {(strip) => (
              <Strip strip={strip} status="ARR" myPosition={myPosition} selectable={true} />
            )}
          </SortableBay>
        </div>

      </div>

      {/* ── Col 3: STARTUP / PUSH BACK / DE-ICE ── */}
      <div className={col}>

        <div className={lockedHeader}>
          <span className={lockedLabel}>STARTUP</span>
        </div>
        <SortableBay
          strips={startupStrips}
          bayId="STARTUP"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[40%] ${scrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className={lockedHeader}>
          <span className={lockedLabel}>PUSH BACK</span>
        </div>
        <SortableBay
          strips={pushStrips}
          bayId="PUSHBACK"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[30%] ${scrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className={lockedHeader}>
          <span className={lockedLabel}>DE-ICE</span>
        </div>
        <SortableBay
          strips={deIceStrips}
          bayId="DE-ICE"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`flex-1 ${scrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

      </div>

      {/* ── Col 4: SAS / NORWEGIAN / OTHERS (UNCLEARED) ── */}
      <div className={col}>

        <div className={(clrDelActive ? activeHeader : lockedHeader) + " justify-between"}>
          <span className={clrDelActive ? activeLabel : lockedLabel}>SAS</span>
          <span className="flex gap-1">
            <button className={btn}>NEW</button>
            <button className={btn}>PLANNED</button>
          </span>
        </div>
        <DropIndicatorBay bayId="SAS" className={`h-[40%] ${scrollArea}`}>
          {sasStrips.map(s => (
            <Strip key={s.callsign} strip={s} status={clrDelActive ? "CLR" : "CLX-HALF"} selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className={clrDelActive ? activeHeader : lockedHeader}>
          <span className={clrDelActive ? activeLabel : lockedLabel}>NORWEGIAN</span>
        </div>
        <DropIndicatorBay bayId="NORWEGIAN" className={`h-[30%] ${scrollArea}`}>
          {norStrips.map(s => (
            <Strip key={s.callsign} strip={s} status={clrDelActive ? "CLR" : "CLX-HALF"} selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className={clrDelActive ? activeHeader : lockedHeader}>
          <span className={clrDelActive ? activeLabel : lockedLabel}>OTHERS</span>
        </div>
        <DropIndicatorBay bayId="OTHERS" className={`flex-1 ${scrollArea}`}>
          {otherStrips.map(s => (
            <Strip key={s.callsign} strip={s} status={clrDelActive ? "CLR" : "CLX-HALF"} selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

      </div>

    </div>
    </ViewDndContext>
  );
}
