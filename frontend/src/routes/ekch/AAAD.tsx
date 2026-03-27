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
import { CLS_BTN, CLS_BTN_BLUE, CLS_SCROLLBAR } from "@/components/strip/shared";
import { NewIfrDialog } from "@/components/strip/NewIfrDialog";
import { PlannedDialog } from "@/components/strip/PlannedDialog";

// Shared header styles
const pageWrapper   = "bg-[#A9A9A9] w-screen h-[95.28vh] flex justify-center justify-items-center gap-2";
const header        = "bg-[#393939] h-10 flex items-center px-2 shrink-0";
const label         = "text-white font-bold text-lg";
const primaryHeader = "bg-primary h-10 flex items-center px-2 shrink-0";
const primaryLabel  = "text-white font-bold text-lg";
const colSep        = "border-t-4 border-[#A9A9A9]";
const scrollArea       = `w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto ${CLS_SCROLLBAR}`;
const scrollAreaBottom = `w-full bg-[#555355] p-1 flex flex-col justify-end gap-px overflow-y-auto ${CLS_SCROLLBAR}`;
const col           = "flex-1 h-full bg-[#555355] flex flex-col min-w-0";
const tabBar        = "flex shrink-0 border-t-8 border-[#A9A9A9]";
const tabBtn        = "flex-1 bg-[#393939] text-white font-bold text-sm border border-white hover:bg-[#4a4a4a]";
const btn           = CLS_BTN;
const btnBlue       = CLS_BTN_BLUE;

export default function AAAD() {
  const myPosition  = useMyPosition();
  const messages    = useMessages();
  const [composeOpen, setComposeOpen] = useState(false);
  const [arrOpen, setArrOpen] = useState(false);
  const [newOpen, setNewOpen] = useState(false);
  const [plannedOpen, setPlannedOpen] = useState(false);

  const delOnline = useDelOnline();
  // When DEL is online, APRON is not responsible for clearances → CLR/DEL panel is inactive.
  // When DEL is offline, APRON handles clearances → CLR/DEL panel is active.
  const clrDelActive = !delOnline;

  const finalStrips   = useFinalStrips().filter(isFlight).sort((a, b) => b.sequence - a.sequence);
  const rwyArrStrips  = useRwyArrStrips().filter(isFlight).sort((a, b) => b.sequence - a.sequence);
  const standStrips   = useStandStrips().filter(isFlight).sort((a, b) => b.sequence - a.sequence);
  const twyDepUpr    = useTaxiDepStrips().sort((a, b) => b.sequence - a.sequence);
  const twyDepLwr    = useTaxiDepLwrStrips().sort((a, b) => b.sequence - a.sequence);
  const twyArrStrips  = useTaxiArrStrips().sort((a, b) => b.sequence - a.sequence);
  const startupStrips = useClearedStrips().sort((a, b) => a.sequence - b.sequence);
  const pushStrips    = usePushbackStrips().filter(isFlight).sort((a, b) => b.sequence - a.sequence);
  const deIceStrips   = useDeIceStrips().filter(isFlight).sort((a, b) => b.sequence - a.sequence);
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
    "TWY-DEP-UPR": { strips: twyDepUpr,    targetBay: Bay.Taxi,     descending: true },
    "TWY-DEP-LWR": { strips: twyDepLwr,    targetBay: Bay.TaxiLwr,  descending: true },
    "TWY-ARR":     { strips: twyArrStrips, targetBay: Bay.TwyArr,   descending: true },
    "STAND":       { strips: standStrips,  targetBay: Bay.Stand,    descending: true },
    "STARTUP":     { strips: startupStrips, targetBay: Bay.Cleared },
    "PUSHBACK":    { strips: pushStrips,    targetBay: Bay.Push,     descending: true },
    "DE-ICE":      { strips: deIceStrips,   targetBay: Bay.DeIce,    descending: true },
  };

  const transferRules: Record<string, string[]> = {
    "TWY-DEP-UPR": ["TWY-DEP-LWR", "TWY-ARR", "STARTUP", "PUSHBACK", "DE-ICE"],
    "TWY-DEP-LWR": ["TWY-DEP-UPR", "TWY-ARR", "STARTUP", "PUSHBACK", "DE-ICE"],
    "TWY-ARR":     ["TWY-DEP-UPR", "TWY-DEP-LWR", "STAND", "STARTUP", "PUSHBACK"],
    "STAND":       ["TWY-ARR"],
    "STARTUP":     ["TWY-DEP-UPR", "TWY-DEP-LWR", "TWY-ARR", "PUSHBACK", "DE-ICE"],
    "PUSHBACK":    ["TWY-DEP-UPR", "TWY-DEP-LWR", "TWY-ARR", "STARTUP", "DE-ICE"],
    "DE-ICE":      ["TWY-DEP-UPR", "TWY-DEP-LWR", "STARTUP", "PUSHBACK"],
  };

  const statusForBay: Record<string, StripStatus> = {
    "TWY-DEP-UPR": "TAXI-DEP",
    "TWY-DEP-LWR": "TAXI-DEP",
    "TWY-ARR":  "ARR",
    "STAND":    "ARR",
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

        <div className={`${header} ${colSep} justify-between`}>
          <span className={label}>FINAL</span>
          <button className={btn} onClick={() => setArrOpen(true)}>ARR</button>
        </div>
        <DropIndicatorBay bayId="FINAL" className={`h-[25%] ${scrollAreaBottom}`}>
          {finalStrips.map(s => (
            <Strip key={s.callsign} strip={s} status="HALF" halfStripVariant="LOCKED-ARR" selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className={`${header} ${colSep}`}>
          <span className={label}>RWY ARR</span>
        </div>
        <DropIndicatorBay bayId="RWY-ARR" className={`h-[30%] ${scrollAreaBottom}`}>
          {rwyArrStrips.map(s => (
            <Strip key={s.callsign} strip={s} status="ARR" selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className={`${header} ${colSep}`}>
          <span className={label}>STAND</span>
        </div>
        <SortableBay
          strips={standStrips}
          bayId="STAND"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`flex-1 ${scrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="ARR" myPosition={myPosition} />
          )}
        </SortableBay>

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

        <div className={header + " justify-between"}>
          <span className={label}>TWY DEP</span>
          <span className="flex gap-1">
            <button className={btn} onClick={() => setNewOpen(true)}>NEW</button>
            <button className={btn} onClick={() => setPlannedOpen(true)}>PLANNED</button>
            <MemAidButton bay={Bay.Taxi} className={btnBlue} />
          </span>
        </div>
        {/* TWY DEP-UPR (intermediate hold short, TAXI bay) */}
        <SortableBay
          strips={twyDepUpr}
          bayId="TWY-DEP-UPR"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[30%] ${scrollAreaBottom}`}
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
          className={`h-[30%] ${scrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="TAXI-DEP" myPosition={myPosition} width={APN_TAXI_DEP_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>

        <div className={`${header} ${colSep}`}>
          <span className={label}>TWY ARR</span>
          <span className="ml-auto">
            <MemAidButton bay={Bay.TwyArr} className={btnBlue} />
          </span>
        </div>
        <SortableBay
          strips={twyArrStrips}
          bayId="TWY-ARR"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`flex-1 ${scrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="ARR" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

      </div>

      {/* ── Col 3: STARTUP / PUSH BACK / DE-ICE ── */}
      <div className={col}>

        <div className={header}>
          <span className={label}>STARTUP</span>
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

        <div className={`${header} ${colSep}`}>
          <span className={label}>PUSH BACK</span>
        </div>
        <SortableBay
          strips={pushStrips}
          bayId="PUSHBACK"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[30%] ${scrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className={`${header} ${colSep}`}>
          <span className={label}>DE-ICE</span>
        </div>
        <SortableBay
          strips={deIceStrips}
          bayId="DE-ICE"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`flex-1 ${scrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

      </div>

      {/* ── Col 4: SAS / NORWEGIAN / OTHERS (UNCLEARED) ── */}
      <div className={col}>

        <div className={`${header} justify-between`}>
          <span className={label}>SAS</span>
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

        <div className={`${header} ${colSep}`}>
          <span className={label}>NORWEGIAN</span>
        </div>
        <DropIndicatorBay bayId="NORWEGIAN" className={`h-[30%] ${scrollArea}`}>
          {norStrips.map(s => (
            <Strip key={s.callsign} strip={s} status={clrDelActive ? "CLR" : "CLX-HALF"} selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className={`${header} ${colSep}`}>
          <span className={label}>OTHERS</span>
        </div>
        <DropIndicatorBay bayId="OTHERS" className={`flex-1 ${scrollArea}`}>
          {otherStrips.map(s => (
            <Strip key={s.callsign} strip={s} status={clrDelActive ? "CLR" : "CLX-HALF"} selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

      </div>

    </div>
    <NewIfrDialog open={newOpen} onOpenChange={setNewOpen} />
    <PlannedDialog open={plannedOpen} onOpenChange={setPlannedOpen} />
    </ViewDndContext>
  );
}
