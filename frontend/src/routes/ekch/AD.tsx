import { Strip } from "@/components/strip/Strip.tsx";
import { DeiceLaneButton, MemAidButton } from "@/components/strip/TacticalButtons.tsx";
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
  useTaxiArrStrips,
  useTaxiDepStrips,
  useTaxiDepLwrStrips,
  isFlight,
} from "@/store/airports/ekch.ts";
import type { AnyStrip, StripRef } from "@/api/models.ts";
import { Bay } from "@/api/models.ts";
import type { StripStatus } from "@/components/strip/types.ts";
import { SortableBay } from "@/components/bays/SortableBay.tsx";
import { ViewDndContext } from "@/components/bays/ViewDndContext.tsx";
import { allBayTransferRules } from "@/components/bays/stripMovement";
import { StripListPopup } from "@/components/StripListPopup.tsx";
import { arrivalSortModes } from "@/lib/stripSortModes";
import { useState } from "react";
import { APN_TAXI_DEP_STRIP_WIDTH } from "@/components/strip/ApnTaxiDepStrip.tsx";
import { CLS_BTN_MISSED, CLS_BTN_BLUE, CLS_LABEL, CLS_BTN_NEW, CLS_BTN_LANE, CLS_BTN_PLANNED, CLS_BTN_ARR } from "@/components/strip/shared";
import { NewIfrDialog } from "@/components/strip/NewIfrDialog";
import { PlannedDialog } from "@/components/strip/PlannedDialog";
import { getDeiceHeaderArea, isDeiceHeaderTacticalStrip } from "@/lib/deiceLane";
import { useDeiceBayDrop } from "@/lib/deiceBayDrop";
import { MessageStrip } from "@/components/strip/MessageStrip.tsx";
import { MessageComposeDialog } from "@/components/MessageComposeDialog.tsx";

const btnBlue = CLS_BTN_BLUE;

import { useBayResize } from "@/components/bays/useBayResize";
import { BayResizeHandle } from "@/components/bays/BayResizeHandle";

// Default bay heights (% of column); the last bay of each column fills the rest.
const COL1_DEFAULTS = { messages: 15, final: 25, rwyArr: 20 };
const COL2_DEFAULTS = { deIceV: 7.4, deIceB: 7.4, twyDepUpr: 30 };
const COL3_DEFAULTS = { startup: 40 };
const COL4_DEFAULTS = { clrDel: 40, norwegian: 30 };

export default function AD() {
  const myPosition  = useMyPosition();
  const messages    = useMessages();
  const [composeOpen, setComposeOpen] = useState(false);
  const [arrOpen, setArrOpen] = useState(false);
  const [newOpen, setNewOpen] = useState(false);
  const [plannedOpen, setPlannedOpen] = useState(false);
  const col1Resize = useBayResize("ad-bay-heights-col1", COL1_DEFAULTS);
  const col2Resize = useBayResize("ad-bay-heights-col2", COL2_DEFAULTS);
  const col3Resize = useBayResize("ad-bay-heights-col3", COL3_DEFAULTS);
  const col4Resize = useBayResize("ad-bay-heights-col4", COL4_DEFAULTS);

  const delOnline = useDelOnline();
  // When DEL is online, APRON is not responsible for clearances → CLR/DEL panel is inactive.
  // When DEL is offline, APRON handles clearances → CLR/DEL panel is active.
  const clrDelActive = !delOnline;

  const finalStrips   = useFinalStrips().sort((a, b) => b.sequence - a.sequence);
  const rwyArrStrips  = useRwyArrStrips().sort((a, b) => b.sequence - a.sequence);
  const twyDepUpr    = useTaxiDepStrips().sort((a, b) => b.sequence - a.sequence);
  const twyDepLwr    = useTaxiDepLwrStrips().sort((a, b) => b.sequence - a.sequence);
  const twyArrStrips  = useTaxiArrStrips().sort((a, b) => b.sequence - a.sequence);
  const startupStrips = useClearedStrips().sort((a, b) => b.sequence - a.sequence);
  const deIceStrips   = useDeIceStrips().sort((a, b) => b.sequence - a.sequence);
  const deIceVStrips  = deIceStrips.filter((strip) => {
    if (isFlight(strip)) return strip.deice_platform?.trim().toUpperCase() === "V";
    return !isDeiceHeaderTacticalStrip(strip) || getDeiceHeaderArea(strip) !== "B";
  });
  const deIceBStrips  = deIceStrips.filter((strip) => {
    if (isFlight(strip)) return strip.deice_platform?.trim().toUpperCase() !== "V";
    return isDeiceHeaderTacticalStrip(strip) && getDeiceHeaderArea(strip) === "B";
  });
  const pushStrips    = usePushbackStrips().sort((a, b) => b.sequence - a.sequence);
  const otherStrips   = useOtherBayStrips().sort((a, b) => a.sequence - b.sequence);
  const sasStrips     = useSasBayStrips().sort((a, b) => a.sequence - b.sequence);
  const norStrips     = useNorwegianBayStrips().sort((a, b) => a.sequence - b.sequence);

  const inboundStrips = useInboundStrips();

  const updateOrder       = useWebSocketStore(state => state.updateOrder);
  const move              = useWebSocketStore(state => state.move);
  const moveTacticalStrip = useWebSocketStore(state => state.moveTacticalStrip);
  const pickupStrip       = useWebSocketStore(state => state.pickupStrip);
  const handleDeiceBayDrop = useDeiceBayDrop();

  const arrSortModes = arrivalSortModes;

  const bayStripMap = {
    "DE-ICE-V":    { strips: deIceVStrips,  targetBay: Bay.DeIce,    descending: true },
    "DE-ICE-B":    { strips: deIceBStrips,  targetBay: Bay.DeIce,    descending: true },
    "TWY-DEP-UPR": { strips: twyDepUpr,    targetBay: Bay.Taxi,     descending: true },
    "TWY-DEP-LWR": { strips: twyDepLwr,    targetBay: Bay.TaxiLwr,  descending: true },
    "TWY-ARR":     { strips: twyArrStrips, targetBay: Bay.TwyArr,   descending: true },
    "STARTUP":     { strips: startupStrips, targetBay: Bay.Cleared, descending: true },
    "PUSHBACK":    { strips: pushStrips,    targetBay: Bay.Push,     descending: true },
    "FINAL":       { strips: finalStrips,   targetBay: Bay.Final,    descending: true },
    "RWY-ARR":     { strips: rwyArrStrips,  targetBay: Bay.RwyArr,   descending: true },
    "CLRDEL":      { strips: sasStrips,     targetBay: Bay.NotCleared },
    "NORWEGIAN":   { strips: norStrips,     targetBay: Bay.NotCleared },
    "OTHERS":      { strips: otherStrips,   targetBay: Bay.NotCleared },
  };

  const transferRules = allBayTransferRules(Object.keys(bayStripMap));

  const statusForBay: Record<string, StripStatus> = {
    "DE-ICE-V":    "TAXI-DEP",
    "DE-ICE-B":    "TAXI-DEP",
    "TWY-DEP-UPR": "TAXI-DEP",
    "TWY-DEP-LWR": "TAXI-DEP",
    "TWY-ARR":  "ARR",
    "STARTUP":  "PUSH",
    "PUSHBACK": "PUSH",
  };

  return (
    <ViewDndContext
      bayStripMap={bayStripMap}
      transferRules={transferRules}
      onReorder={(activeRef: StripRef, insertAfter: StripRef | null) => {
        if (activeRef.kind === "tactical") moveTacticalStrip(activeRef.id!, insertAfter);
        else updateOrder(activeRef.callsign!, insertAfter);
      }}
      onMove={(activeRef, bay, insertAfter) => {
        if (activeRef.kind === "tactical") moveTacticalStrip(activeRef.id!, insertAfter ?? null, bay);
        else move(activeRef.callsign!, bay, false, false, insertAfter);
      }}
      onBayDrop={handleDeiceBayDrop}
      renderDragOverlay={(strip: AnyStrip) => {
        if (!isFlight(strip)) return <Strip strip={strip} width={APN_TAXI_DEP_STRIP_WIDTH} />;
        const bayEntry = Object.entries(bayStripMap).find(([, c]) =>
          c.strips.some(s => isFlight(s) && s.callsign === strip.callsign)
        );
        if (!bayEntry) return null;
        const [bayId] = bayEntry;
        if (bayId === "FINAL") return <Strip strip={strip} status="HALF" halfStripVariant="LOCKED-ARR" myPosition={myPosition} />;
        if (bayId === "RWY-ARR") return <Strip strip={strip} status="ARR" myPosition={myPosition} />;
        if (["CLRDEL", "NORWEGIAN", "OTHERS"].includes(bayId)) {
          return <Strip strip={strip} status={clrDelActive ? "CLR" : "CLX-HALF"} myPosition={myPosition} />;
        }
        return <Strip strip={strip} status={statusForBay[bayId]} myPosition={myPosition} />;
      }}
    >
    <div className="bay-page-wrapper">

      {/* ── Col 1: MESSAGES / FINAL (locked) / RWY ARR (locked) / TWY ARR ── */}
      <div style={col1Resize.columnStyle} className="bay-col-flex">

        <div className="bay-col-header-teal justify-between">
          <span className={CLS_LABEL}>MESSAGES</span>
          <button className={CLS_BTN_MISSED} onClick={() => setComposeOpen(true)}>FREE TEXT</button>
        </div>
        <div className="h-[var(--bay-h-messages)] bay-scroll-area">
          {messages.map(msg => (
            <MessageStrip key={msg.id} msg={msg} />
          ))}
        </div>
        <MessageComposeDialog open={composeOpen} onClose={() => setComposeOpen(false)} />

        <div className="bay-col-header bay-col-sep justify-between">
          <BayResizeHandle {...col1Resize.handleProps("messages")} />
          <span className={CLS_LABEL}>FINAL</span>
          <button className={CLS_BTN_ARR} onClick={() => setArrOpen(true)}>ARR</button>
        </div>
        <SortableBay strips={finalStrips} bayId="FINAL" isDragDisabled={(strip) => !!strip.owner && strip.owner !== myPosition} standalone={false} className="h-[var(--bay-h-final)] bay-scroll-area-bottom">
          {(strip) => <Strip strip={strip} status="HALF" halfStripVariant="LOCKED-ARR" selectable={false} myPosition={myPosition} />}
        </SortableBay>

        <div className="bay-col-header bay-col-sep">
          <BayResizeHandle {...col1Resize.handleProps("final")} />
          <span className={CLS_LABEL}>RWY ARR</span>
        </div>
        <SortableBay strips={rwyArrStrips} bayId="RWY-ARR" isDragDisabled={(strip) => !!strip.owner && strip.owner !== myPosition} standalone={false} className="h-[var(--bay-h-rwyArr)] bay-scroll-area-bottom">
          {(strip) => <Strip strip={strip} status="ARR" selectable={false} myPosition={myPosition} />}
        </SortableBay>

        <div className="bay-col-header bay-col-sep">
          <BayResizeHandle {...col1Resize.handleProps("rwyArr")} />
          <span className={CLS_LABEL}>TWY ARR</span>
          <span className="ml-auto">
            <MemAidButton bay={Bay.TwyArr} className={btnBlue} />
          </span>
        </div>
        <SortableBay
          strips={twyArrStrips}
          bayId="TWY-ARR"
          isDragDisabled={(strip) => !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="flex-1 bay-scroll-area-bottom"
        >
          {(strip) => (
            <Strip strip={strip} status="ARR" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        {arrOpen && (
          <StripListPopup
            title="ARR"
            strips={inboundStrips}
            sortModes={arrSortModes}
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

      {/* ── Col 2: DE-ICE V / DE-ICE B / TWY DEP (UPR + LWR) ── */}
      <div style={col2Resize.columnStyle} className="bay-col-flex">

        <div className="bay-col-header">
          <span className={CLS_LABEL}>DE-ICE V</span>
        </div>
        <SortableBay
          strips={deIceVStrips}
          bayId="DE-ICE-V"
          isDragDisabled={(strip) => !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="h-[var(--bay-h-deIceV)] bay-scroll-area-bottom"
        >
          {(strip) => (
            <Strip strip={strip} status="TAXI-DEP" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className="bay-col-header bay-col-sep justify-between">
          <BayResizeHandle {...col2Resize.handleProps("deIceV")} />
          <span className={CLS_LABEL}>DE-ICE B</span>
          <span className="flex gap-0.5">
            <DeiceLaneButton area="B" lane={1} frequency="131.650" className={CLS_BTN_LANE} />
            <DeiceLaneButton area="B" lane={2} frequency="131.975" className={CLS_BTN_LANE} />
          </span>
        </div>
        <SortableBay
          strips={deIceBStrips}
          bayId="DE-ICE-B"
          isDragDisabled={(strip) => !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="h-[var(--bay-h-deIceB)] bay-scroll-area-bottom"
        >
          {(strip) => (
            <Strip strip={strip} status="TAXI-DEP" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className="bay-col-header bay-col-sep justify-between">
          <BayResizeHandle {...col2Resize.handleProps("deIceB")} />
          <span className={CLS_LABEL}>TWY DEP</span>
          <span className="flex gap-0.5">
            <MemAidButton bay={Bay.Taxi} className={btnBlue} />
          </span>
        </div>
        {/* TWY DEP-UPR (intermediate hold short, TAXI bay) */}
        <SortableBay
          strips={twyDepUpr}
          bayId="TWY-DEP-UPR"
          isDragDisabled={(strip) => !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="h-[var(--bay-h-twyDepUpr)] bay-scroll-area-bottom"
        >
          {(strip) => (
            <Strip strip={strip} status="TAXI-DEP" myPosition={myPosition} width={APN_TAXI_DEP_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>

        {/* TW / TE / GW / GE bay selector tabs */}
        <div className="bay-tab-bar relative">
          <BayResizeHandle {...col2Resize.handleProps("twyDepUpr")} />
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
          isDragDisabled={(strip) => !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="flex-1 bay-scroll-area-bottom bay-no-top-shadow"
        >
          {(strip) => (
            <Strip strip={strip} status="TAXI-DEP" myPosition={myPosition} width={APN_TAXI_DEP_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>

      </div>

      {/* ── Col 3: STARTUP / PUSH BACK ── */}
      <div style={col3Resize.columnStyle} className="bay-col-flex">

        <div className="bay-col-header">
          <span className={CLS_LABEL}>STARTUP</span>
        </div>
        <SortableBay
          strips={startupStrips}
          bayId="STARTUP"
          isDragDisabled={(strip) => !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="h-[var(--bay-h-startup)] bay-scroll-area-bottom"
        >
          {(strip) => (
            <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className="bay-col-header bay-col-sep">
          <BayResizeHandle {...col3Resize.handleProps("startup")} />
          <span className={CLS_LABEL}>PUSH BACK</span>
        </div>
        <SortableBay
          strips={pushStrips}
          bayId="PUSHBACK"
          isDragDisabled={(strip) => !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="flex-1 bay-scroll-area-bottom"
        >
          {(strip) => (
            <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

      </div>

      {/* ── Col 4: SAS / NORWEGIAN / OTHERS (UNCLEARED) ── */}
      <div style={col4Resize.columnStyle} className="bay-col-flex">

        <div className="bay-col-header">
          <span className={CLS_LABEL}>SAS</span>
        </div>
        <SortableBay strips={sasStrips} bayId="CLRDEL" standalone={false} className="h-[var(--bay-h-clrDel)] bay-scroll-area">
          {(strip) => <Strip strip={strip} status={clrDelActive ? "CLR" : "CLX-HALF"} selectable={false} myPosition={myPosition} />}
        </SortableBay>

        <div className="bay-col-header bay-col-sep">
          <BayResizeHandle {...col4Resize.handleProps("clrDel")} />
          <span className={CLS_LABEL}>NORWEGIAN</span>
        </div>
        <SortableBay strips={norStrips} bayId="NORWEGIAN" standalone={false} className="h-[var(--bay-h-norwegian)] bay-scroll-area">
          {(strip) => <Strip strip={strip} status={clrDelActive ? "CLR" : "CLX-HALF"} selectable={false} myPosition={myPosition} />}
        </SortableBay>

        <div className="bay-col-header bay-col-sep justify-between">
          <BayResizeHandle {...col4Resize.handleProps("norwegian")} />
          <span className={CLS_LABEL}>OTHERS</span>
          <span className="flex gap-0.5">
            <button className={CLS_BTN_NEW} onClick={() => setNewOpen(true)}>NEW</button>
            <button className={CLS_BTN_PLANNED} onClick={() => setPlannedOpen(true)}>PLANNED</button>
          </span>
        </div>
        <SortableBay strips={otherStrips} bayId="OTHERS" standalone={false} className="flex-1 bay-scroll-area">
          {(strip) => <Strip strip={strip} status={clrDelActive ? "CLR" : "CLX-HALF"} selectable={false} myPosition={myPosition} />}
        </SortableBay>

      </div>

    </div>
    <NewIfrDialog open={newOpen} onOpenChange={setNewOpen} />
    <PlannedDialog open={plannedOpen} onOpenChange={setPlannedOpen} />
    </ViewDndContext>
  );
}
