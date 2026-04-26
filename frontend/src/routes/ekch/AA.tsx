import { Strip } from "@/components/strip/Strip.tsx";
import { MemAidButton } from "@/components/strip/TacticalButtons.tsx";
import { useMyPosition, useWebSocketStore, useDelOnline } from "@/store/store-hooks.ts";
import {
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
import { CLS_BTN, CLS_BTN_BLUE, CLS_LABEL } from "@/components/strip/shared";
import { NewIfrDialog } from "@/components/strip/NewIfrDialog";
import { PlannedDialog } from "@/components/strip/PlannedDialog";

const btn     = CLS_BTN;
const btnBlue = CLS_BTN_BLUE;

export default function AA() {
  const myPosition  = useMyPosition();
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
  const twyDepUpr     = useTaxiDepStrips().sort((a, b) => b.sequence - a.sequence);
  const twyDepLwr     = useTaxiDepLwrStrips().sort((a, b) => b.sequence - a.sequence);
  const twyArrStrips  = useTaxiArrStrips().sort((a, b) => b.sequence - a.sequence);
  const pushStrips    = usePushbackStrips().filter(isFlight).sort((a, b) => b.sequence - a.sequence);
  const deIceStrips   = useDeIceStrips().filter(isFlight).sort((a, b) => b.sequence - a.sequence);
  const otherStrips   = useOtherBayStrips().sort((a, b) => a.sequence - b.sequence);
  const sasStrips     = useSasBayStrips().sort((a, b) => a.sequence - b.sequence);
  const norStrips     = useNorwegianBayStrips().sort((a, b) => a.sequence - b.sequence);

  const inboundStrips = useInboundStrips();

  const updateOrder       = useWebSocketStore(state => state.updateOrder);
  const move              = useWebSocketStore(state => state.move);
  const moveTacticalStrip = useWebSocketStore(state => state.moveTacticalStrip);
  const pickupStrip       = useWebSocketStore(state => state.pickupStrip);

  const arrSortModes: SortMode<FrontendStrip>[] = [
    { key: "ETA",      label: "ETA",      compareFn: (a, b) => a.eldt.localeCompare(b.eldt) },
    { key: "CALLSIGN", label: "CALLSIGN", compareFn: (a, b) => a.callsign.localeCompare(b.callsign) },
    { key: "ADEP",     label: "ADEP",     compareFn: (a, b) => a.origin.localeCompare(b.origin) },
  ];

  const bayStripMap = {
    "TWY-DEP-UPR": { strips: twyDepUpr,    targetBay: Bay.Taxi,    descending: true },
    "TWY-DEP-LWR": { strips: twyDepLwr,    targetBay: Bay.TaxiLwr, descending: true },
    "TWY-ARR":     { strips: twyArrStrips, targetBay: Bay.TwyArr,  descending: true },
    "STAND":       { strips: standStrips,  targetBay: Bay.Stand,   descending: true },
    "PUSHBACK":    { strips: pushStrips,   targetBay: Bay.Push,    descending: true },
    "DE-ICE":      { strips: deIceStrips,  targetBay: Bay.DeIce,   descending: true },
  };

  const transferRules: Record<string, string[]> = {
    "TWY-DEP-UPR": ["TWY-DEP-LWR", "TWY-ARR", "PUSHBACK", "DE-ICE"],
    "TWY-DEP-LWR": ["TWY-DEP-UPR", "TWY-ARR", "PUSHBACK", "DE-ICE"],
    "TWY-ARR":     ["TWY-DEP-UPR", "TWY-DEP-LWR", "STAND", "PUSHBACK"],
    "STAND":       ["TWY-ARR"],
    "PUSHBACK":    ["TWY-DEP-UPR", "TWY-DEP-LWR", "TWY-ARR", "DE-ICE"],
    "DE-ICE":      ["TWY-DEP-UPR", "TWY-DEP-LWR", "PUSHBACK"],
  };

  const statusForBay: Record<string, StripStatus> = {
    "TWY-DEP-UPR": "TAXI-DEP",
    "TWY-DEP-LWR": "TAXI-DEP",
    "TWY-ARR":  "ARR",
    "STAND":    "ARR",
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
      onMove={(activeRef, bay) => {
        if (activeRef.kind === "tactical") moveTacticalStrip(activeRef.id!, null, bay);
        else move(activeRef.callsign!, bay);
      }}
      renderDragOverlay={(strip: AnyStrip) => {
        if (!isFlight(strip)) return <Strip strip={strip} width={APN_TAXI_DEP_STRIP_WIDTH} />;
        const bayEntry = Object.entries(bayStripMap).find(([, c]) =>
          c.strips.some(s => isFlight(s) && s.callsign === strip.callsign)
        );
        if (!bayEntry) return null;
        return <Strip strip={strip} status={statusForBay[bayEntry[0]]} myPosition={myPosition} />;
      }}
    >
    <div className="bay-page-wrapper">

      {/* ── Col 1: FINAL (locked) / RWY ARR (locked) / STAND ── */}
      <div className="bay-col-flex">

        <div className="bay-col-header justify-between">
          <span className={CLS_LABEL}>FINAL</span>
          <button className={btn} onClick={() => setArrOpen(true)}>ARR</button>
        </div>
        <DropIndicatorBay bayId="FINAL" className="h-[30%] bay-scroll-area-bottom">
          {finalStrips.map(s => (
            <Strip key={s.callsign} strip={s} status="HALF" halfStripVariant="LOCKED-ARR" selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className="bay-col-header bay-col-sep">
          <span className={CLS_LABEL}>RWY ARR</span>
        </div>
        <DropIndicatorBay bayId="RWY-ARR" className="h-[25%] bay-scroll-area-bottom">
          {rwyArrStrips.map(s => (
            <Strip key={s.callsign} strip={s} status="ARR" selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className="bay-col-header bay-col-sep">
          <span className={CLS_LABEL}>STAND</span>
        </div>
        <SortableBay
          strips={standStrips}
          bayId="STAND"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="flex-1 bay-scroll-area-bottom"
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
            rowStripStatus="HALF"
            rowHalfStripVariant="LOCKED-ARR"
            onRowClick={(strip) => {
              pickupStrip(strip.callsign, Bay.Final);
              setArrOpen(false);
            }}
            onDismiss={() => setArrOpen(false)}
            myPosition={myPosition}
          />
        )}
      </div>

      {/* ── Col 2: DE-ICE / TWY ARR ── */}
      <div className="bay-col-flex">

        <div className="bay-col-header">
          <span className={CLS_LABEL}>DE-ICE</span>
        </div>
        <SortableBay
          strips={deIceStrips}
          bayId="DE-ICE"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="h-[20%] bay-scroll-area-bottom"
        >
          {(strip) => (
            <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className="bay-col-header bay-col-sep justify-between">
          <span className={CLS_LABEL}>TWY ARR</span>
          <MemAidButton bay={Bay.TwyArr} className={btnBlue} />
        </div>
        <SortableBay
          strips={twyArrStrips}
          bayId="TWY-ARR"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="flex-1 bay-scroll-area-bottom"
        >
          {(strip) => (
            <Strip strip={strip} status="ARR" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

      </div>

      {/* ── Col 3: PUSHBACK / TWY DEP (UPR + LWR) ── */}
      <div className="bay-col-flex">

        <div className="bay-col-header">
          <span className={CLS_LABEL}>PUSHBACK</span>
        </div>
        <SortableBay
          strips={pushStrips}
          bayId="PUSHBACK"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="h-[35%] bay-scroll-area-bottom"
        >
          {(strip) => (
            <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className="bay-col-header bay-col-sep justify-between">
          <span className={CLS_LABEL}>TWY DEP</span>
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
          className="h-[25%] bay-scroll-area-bottom"
        >
          {(strip) => (
            <Strip strip={strip} status="TAXI-DEP" myPosition={myPosition} width={APN_TAXI_DEP_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>

        {/* TW / TE / GW / GE bay selector tabs */}
        <div className="bay-tab-bar">
          {["TW", "TE", "GW", "GE"].map(tab => (
            <button
              key={tab}
              className="bay-tab-btn"
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
          className="flex-1 bay-scroll-area-bottom"
        >
          {(strip) => (
            <Strip strip={strip} status="TAXI-DEP" myPosition={myPosition} width={APN_TAXI_DEP_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>

      </div>

      {/* ── Col 4: CLRDEL / NORWEGIAN / OTHERS (UNCLEARED) ── */}
      <div className="bay-col-flex">

        <div className="bay-col-header">
          <span className={CLS_LABEL}>CLRDEL</span>
        </div>
        <DropIndicatorBay bayId="CLRDEL" className="h-[40%] bay-scroll-area">
          {sasStrips.map(s => (
            <Strip key={s.callsign} strip={s} status={clrDelActive ? "CLR" : "CLX-HALF"} selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className="bay-col-header bay-col-sep">
          <span className={CLS_LABEL}>NORWEGIAN</span>
        </div>
        <DropIndicatorBay bayId="NORWEGIAN" className="h-[30%] bay-scroll-area">
          {norStrips.map(s => (
            <Strip key={s.callsign} strip={s} status={clrDelActive ? "CLR" : "CLX-HALF"} selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className="bay-col-header bay-col-sep justify-between">
          <span className={CLS_LABEL}>OTHERS</span>
          <span className="flex gap-1">
            <button className={btn} onClick={() => setNewOpen(true)}>NEW</button>
            <button className={btn} onClick={() => setPlannedOpen(true)}>PLANNED</button>
          </span>
        </div>
        <DropIndicatorBay bayId="OTHERS" className="flex-1 bay-scroll-area">
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
