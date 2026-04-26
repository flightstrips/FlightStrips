import { Strip } from "@/components/strip/Strip.tsx";
import { MemAidButton, CrossingButton, StartButton, LandButton } from "@/components/strip/TacticalButtons.tsx";
import { MessageStrip } from "@/components/strip/MessageStrip.tsx";
import { MessageComposeDialog } from "@/components/MessageComposeDialog.tsx";
import {
  useClearedStrips,
  useFinalStrips,
  useInboundStrips,
  useNonClearedStrips,
  usePushbackStrips,
  useRwyArrStrips,
  useStandStrips,
  useTaxiArrStrips,
  useTaxiDepLwrStrips,
  useDeIceStrips,
  useAirborneStrips,
  useDepartStrips,
  isFlight,
} from "@/store/airports/ekch.ts";
import type { AnyStrip, FrontendStrip, StripRef } from "@/api/models.ts";
import { Bay } from "@/api/models.ts";
import { SortableBay } from "@/components/bays/SortableBay.tsx";
import { ViewDndContext } from "@/components/bays/ViewDndContext.tsx";
import { useWebSocketStore, useMyPosition, useMessages, useDelOnline, useApronOnline } from "@/store/store-hooks.ts";
import { StripListPopup, type SortMode } from "@/components/StripListPopup.tsx";
import { useState } from "react";
import { CLX_CLEARED_STRIP_WIDTH } from "@/components/strip/ClxClearedStrip.tsx";
import { TWY_DEP_STRIP_WIDTH } from "@/components/strip/types";
import { CLS_BTN, CLS_BTN_ORANGE, CLS_BTN_BLUE, CLS_BTN_YELLOW, CLS_LABEL } from "@/components/strip/shared";
import { NewIfrDialog } from "@/components/strip/NewIfrDialog";
import { PlannedDialog } from "@/components/strip/PlannedDialog";

// Column widths
const W_COL_ARR      = "w-[27%]";
const W_COL_DEP      = "w-[28%]";
const W_COL_CLRDEL   = "w-1/4";
const W_COL_STAND    = "w-1/5";

export default function GEGW() {
  const myPosition = useMyPosition();
  const messages   = useMessages();
  const [composeOpen, setComposeOpen] = useState(false);
  const [arrOpen, setArrOpen] = useState(false);
  const [newOpen, setNewOpen] = useState(false);
  const [plannedOpen, setPlannedOpen] = useState(false);

  const finalStrips    = useFinalStrips().sort((a, b) => b.sequence - a.sequence);
  const rwyArrStrips   = useRwyArrStrips().sort((a, b) => b.sequence - a.sequence);
  const twyArrStrips   = useTaxiArrStrips().sort((a, b) => b.sequence - a.sequence);
  const pushStrips     = usePushbackStrips().sort((a, b) => b.sequence - a.sequence);
  const startupStrips  = useClearedStrips();
  const twyDepDesc     = useTaxiDepLwrStrips().sort((a, b) => b.sequence - a.sequence);
  const rwyDepStrips   = useDepartStrips().sort((a, b) => b.sequence - a.sequence);
  const airborneStrips = useAirborneStrips().sort((a, b) => b.sequence - a.sequence);
  const deIceStrips    = useDeIceStrips().sort((a, b) => b.sequence - a.sequence);
  const standStrips    = useStandStrips().sort((a, b) => b.sequence - a.sequence);

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

  const delOnline   = useDelOnline();
  const apronOnline = useApronOnline();
  // CTWR is responsible for clearances only when neither DEL nor APRON is online.
  const clrDelActive = !delOnline && !apronOnline;

  const nonClearedStrips = useNonClearedStrips();

  const bayStripMap = {
    "STARTUP":  { strips: startupStrips,                    targetBay: Bay.Cleared },
    "PUSHBACK": { strips: pushStrips,                       targetBay: Bay.Push,      descending: true },
    "TWY-DEP":  { strips: twyDepDesc,                       targetBay: Bay.TaxiLwr,   descending: true },
    "RWY-DEP":  { strips: rwyDepStrips,                     targetBay: Bay.Depart,    descending: true },
    "AIRBORNE": { strips: airborneStrips,                   targetBay: Bay.Airborne,  descending: true },
    "DE-ICE":   { strips: deIceStrips,                      targetBay: Bay.DeIce,     descending: true },
    "STAND":    { strips: standStrips,                      targetBay: Bay.Stand,     descending: true },
    "FINAL":    { strips: finalStrips.filter(isFlight),     targetBay: Bay.Final,     descending: true },
    "RWY-ARR":  { strips: rwyArrStrips.filter(isFlight),    targetBay: Bay.RwyArr,    descending: true },
    "TWY-ARR":  { strips: twyArrStrips,                     targetBay: Bay.TwyArr,    descending: true },
  };

  const transferRules: Record<string, string[]> = {
    "STARTUP":  ["PUSHBACK", "TWY-DEP", "DE-ICE", "STAND"],
    "PUSHBACK": ["STARTUP",  "TWY-DEP", "DE-ICE", "STAND"],
    "TWY-DEP":  ["STARTUP",  "PUSHBACK", "DE-ICE", "STAND"],
    "RWY-DEP":  ["TWY-DEP",  "AIRBORNE"],
    "AIRBORNE": ["RWY-DEP"],
    "DE-ICE":   ["STARTUP",  "PUSHBACK", "TWY-DEP"],
    "STAND":    ["STARTUP",  "PUSHBACK", "TWY-DEP"],
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
        if (!isFlight(strip)) return <Strip strip={strip} width={CLX_CLEARED_STRIP_WIDTH} />;
        if (strip.bay === Bay.Cleared)   return <Strip strip={strip} status="PUSH" myPosition={myPosition} />;
        if (strip.bay === Bay.Push)      return <Strip strip={strip} status="HALF" halfStripVariant="APN-PUSH" myPosition={myPosition} />;
        if (strip.bay === Bay.TaxiLwr)   return <div style={{ width: TWY_DEP_STRIP_WIDTH }}><Strip strip={strip} status="TWY-DEP" myPosition={myPosition} fullWidth /></div>;
        if (strip.bay === Bay.Depart)    return <div style={{ width: TWY_DEP_STRIP_WIDTH }}><Strip strip={strip} status="TWY-DEP" myPosition={myPosition} fullWidth /></div>;
        if (strip.bay === Bay.Airborne)  return <div style={{ width: TWY_DEP_STRIP_WIDTH }}><Strip strip={strip} status="TWY-DEP" myPosition={myPosition} fullWidth /></div>;
        if (strip.bay === Bay.DeIce)     return <Strip strip={strip} status="PUSH" myPosition={myPosition} />;
        if (strip.bay === Bay.Stand)     return <Strip strip={strip} status="ARR" myPosition={myPosition} />;
        if (strip.bay === Bay.Final)     return <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} />;
        if (strip.bay === Bay.RwyArr)    return <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} />;
        if (strip.bay === Bay.TwyArr)    return <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} />;
        return null;
      }}
    >
    <div className="bay-page-wrapper">

      {/* Column 1 (27%) – FINAL + RWY ARR + TWY ARR */}
      <div className={`${W_COL_ARR} bay-col`}>
        <div className="bay-col-header justify-between">
          <span className={CLS_LABEL}>FINAL</span>
          <button className={CLS_BTN} onClick={() => setArrOpen(true)}>ARR</button>
        </div>
        <SortableBay
          strips={finalStrips.filter(isFlight)}
          bayId="FINAL"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="h-[25%] bay-scroll-area-bottom"
        >
          {(strip) => (
            <Strip strip={strip} status="FINAL-ARR" selectable={false} myPosition={myPosition} />
          )}
        </SortableBay>

        <div className="bay-col-header bay-col-sep">
          <span className={CLS_LABEL}>RWY ARR</span>
        </div>
        <SortableBay
          strips={rwyArrStrips.filter(isFlight)}
          bayId="RWY-ARR"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="h-[20%] bay-scroll-area-dark"
        >
          {(strip) => (
            <Strip strip={strip} status="FINAL-ARR" selectable={false} myPosition={myPosition} />
          )}
        </SortableBay>

        <div className="bay-col-header bay-col-sep justify-between">
          <span className={CLS_LABEL}>TWY ARR</span>
          <span className="flex gap-1">
            <MemAidButton bay={Bay.TwyArr} className={CLS_BTN_BLUE} />
            <LandButton bay={Bay.TwyArr} className={CLS_BTN_ORANGE} />
            <StartButton bay={Bay.TwyArr} className={CLS_BTN_ORANGE} />
            <CrossingButton bay={Bay.TwyArr} className={CLS_BTN_YELLOW} />
          </span>
        </div>
        <SortableBay
          strips={twyArrStrips}
          bayId="TWY-ARR"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="flex-1 bay-scroll-area-bottom"
        >
          {(strip) => (
            <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} />
          )}
        </SortableBay>

        {arrOpen && (
          <StripListPopup
            title="ARR"
            strips={inboundStrips}
            sortModes={arrSortModes}
            rowStripStatus="FINAL-ARR"
            onRowClick={(strip) => {
              pickupStrip(strip.callsign, Bay.Final);
              setArrOpen(false);
            }}
            onDismiss={() => setArrOpen(false)}
            myPosition={myPosition}
          />
        )}
      </div>

      {/* Column 2 (28%) – PUSHBACK + TWY DEP + RWY DEP + AIRBORNE */}
      <div className={`${W_COL_DEP} bay-col`}>
        <div className="bay-col-header">
          <span className={CLS_LABEL}>PUSHBACK</span>
        </div>
        <SortableBay
          strips={pushStrips}
          bayId="PUSHBACK"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="h-[12%] bay-scroll-area-bottom"
        >
          {(strip) => (
            <Strip strip={strip} status="HALF" halfStripVariant="APN-PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className="bay-col-header bay-col-sep justify-between">
          <span className={CLS_LABEL}>TWY DEP</span>
          <span className="flex gap-1">
            <button className={CLS_BTN} onClick={() => setNewOpen(true)}>NEW</button>
            <MemAidButton bay={Bay.TaxiLwr} className={CLS_BTN_BLUE} />
            <LandButton bay={Bay.TaxiLwr} className={CLS_BTN_ORANGE} />
            <StartButton bay={Bay.TaxiLwr} className={CLS_BTN_ORANGE} />
            <CrossingButton bay={Bay.TaxiLwr} className={CLS_BTN_YELLOW} />
          </span>
        </div>
        <SortableBay
          strips={twyDepDesc}
          bayId="TWY-DEP"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="h-[35%] bay-scroll-area-bottom"
        >
          {(strip) => (
            <Strip strip={strip} status="TWY-DEP" myPosition={myPosition} width={TWY_DEP_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>

        <div className="bay-col-header bay-col-sep">
          <span className={CLS_LABEL}>RWY DEP</span>
        </div>
        <SortableBay
          strips={rwyDepStrips}
          bayId="RWY-DEP"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="h-[15%] bay-scroll-area-dark"
        >
          {(strip) => (
            <Strip strip={strip} status="TWY-DEP" myPosition={myPosition} width={TWY_DEP_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>

        <div className="bay-col-header bay-col-sep">
          <span className={CLS_LABEL}>AIRBORNE</span>
        </div>
        <SortableBay
          strips={airborneStrips}
          bayId="AIRBORNE"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="flex-1 bay-scroll-area-dark"
        >
          {(strip) => (
            <Strip strip={strip} status="TWY-DEP" myPosition={myPosition} width={TWY_DEP_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>
      </div>

      {/* Column 3 (25%) – STARTUP + DE-ICE A + MESSAGES */}
      <div className={`${W_COL_CLRDEL} bay-col`}>
        <div className="bay-col-header justify-between">
          <span className={CLS_LABEL}>STARTUP</span>
          <button className={CLS_BTN} onClick={() => setNewOpen(true)}>NEW</button>
        </div>
        <SortableBay
          strips={startupStrips}
          bayId="STARTUP"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="h-[33%] bay-scroll-area"
        >
          {(strip) => (
            <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className="bay-col-header bay-col-sep">
          <span className={CLS_LABEL}>DE-ICE A</span>
        </div>
        <SortableBay
          strips={deIceStrips}
          bayId="DE-ICE"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="h-[33%] bay-scroll-area-bottom"
        >
          {(strip) => (
            <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className="bay-col-header-primary bay-col-sep justify-between">
          <span className="text-white font-bold text-lg">MESSAGES</span>
          <span className="flex gap-1">
            <button className={CLS_BTN}>INFO</button>
            <button className={CLS_BTN}>MISC.</button>
            <button className={CLS_BTN}>EQUIP</button>
          </span>
        </div>
        <div className="flex-1 bay-scroll-area">
          {messages.map(msg => (
            <MessageStrip key={msg.id} msg={msg} />
          ))}
        </div>
        <MessageComposeDialog open={composeOpen} onClose={() => setComposeOpen(false)} />
      </div>

      {/* Column 4 (20%) – CLRDEL + STAND */}
      <div className={`${W_COL_STAND} bay-col`}>
        <div className="bay-col-header justify-between">
          <span className={CLS_LABEL}>CLRDEL</span>
          <span className="flex gap-1">
            <button className={CLS_BTN} onClick={() => setNewOpen(true)}>NEW</button>
            <button className={CLS_BTN} onClick={() => setPlannedOpen(true)}>PLANNED</button>
          </span>
        </div>
        <div className="h-[75%] bay-scroll-area">
          {clrDelActive && nonClearedStrips.map(s => (
            <Strip key={s.callsign} strip={s} status="CLR" selectable={false} myPosition={myPosition} fullWidth />
          ))}
        </div>

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
            <Strip strip={strip} status="ARR" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>
      </div>

    </div>
    <NewIfrDialog open={newOpen} onOpenChange={setNewOpen} />
    <PlannedDialog open={plannedOpen} onOpenChange={setPlannedOpen} />
    </ViewDndContext>
  );
}
