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
import type { AnyStrip, StripRef } from "@/api/models.ts";
import { Bay } from "@/api/models.ts";
import { SortableBay } from "@/components/bays/SortableBay.tsx";
import { ViewDndContext } from "@/components/bays/ViewDndContext.tsx";
import { allBayTransferRules } from "@/components/bays/stripMovement";
import { useWebSocketStore, useMyPosition, useMessages, useDelOnline, useApronOnline } from "@/store/store-hooks.ts";
import { StripListPopup } from "@/components/StripListPopup.tsx";
import { arrivalSortModes } from "@/lib/stripSortModes";
import { useState } from "react";
import { CLX_CLEARED_STRIP_WIDTH } from "@/components/strip/ClxClearedStrip.tsx";
import { TWY_DEP_STRIP_WIDTH } from "@/components/strip/types";
import { CLS_BTN_ORANGE, CLS_BTN_BLUE, CLS_BTN_YELLOW, CLS_LABEL, CLS_BTN_NEW, CLS_BTN_PLANNED, CLS_BTN_ARR } from "@/components/strip/shared";
import { NewIfrDialog } from "@/components/strip/NewIfrDialog";
import { PlannedDialog } from "@/components/strip/PlannedDialog";
import { shouldShowInGegwApronBay } from "@/config/ekchStandGroups";
import { GEGW_COLUMN_CLASSES } from "./productionBayLayouts";
import { useBayResize } from "@/components/bays/useBayResize";
import { BayResizeHandle } from "@/components/bays/BayResizeHandle";

// Column widths
const [COL_ARR, COL_DEP, COL_CLRDEL, COL_STAND] = GEGW_COLUMN_CLASSES;

// Default bay heights (% of column); the last bay of each column fills the rest.
const ARR_DEFAULTS = { final: 25, rwyArr: 20 };
const DEP_DEFAULTS = { push: 12, twyDep: 35, rwyDep: 15 };
const CLRDEL_DEFAULTS = { startup: 33, deIce: 33 };
const STAND_DEFAULTS = { clrDel: 75 };

export default function GEGW() {
  const myPosition = useMyPosition();
  const messages   = useMessages();
  const [composeOpen, setComposeOpen] = useState(false);
  const [arrOpen, setArrOpen] = useState(false);
  const [newOpen, setNewOpen] = useState(false);
  const [plannedOpen, setPlannedOpen] = useState(false);
  const arrResize    = useBayResize("gegw-bay-heights-arr", ARR_DEFAULTS);
  const depResize    = useBayResize("gegw-bay-heights-dep", DEP_DEFAULTS);
  const clrDelResize = useBayResize("gegw-bay-heights-clrdel", CLRDEL_DEFAULTS);
  const standResize  = useBayResize("gegw-bay-heights-stand", STAND_DEFAULTS);

  const finalStrips    = useFinalStrips().sort((a, b) => b.sequence - a.sequence);
  const rwyArrStrips   = useRwyArrStrips().sort((a, b) => b.sequence - a.sequence);
  const twyArrStrips   = useTaxiArrStrips().sort((a, b) => b.sequence - a.sequence);
  const allPushStrips  = usePushbackStrips();
  const allStartupStrips = useClearedStrips();
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

  const arrSortModes = arrivalSortModes;

  const delOnline   = useDelOnline();
  const apronOnline = useApronOnline();
  const pushStrips = allPushStrips
    .filter((strip) => shouldShowInGegwApronBay(isFlight(strip) ? strip.stand : undefined, isFlight(strip), apronOnline))
    .sort((a, b) => b.sequence - a.sequence);
  const startupStrips = allStartupStrips
    .filter((strip) => shouldShowInGegwApronBay(isFlight(strip) ? strip.stand : undefined, isFlight(strip), apronOnline))
    .sort((a, b) => b.sequence - a.sequence);
  // CTWR is responsible for clearances only when neither DEL nor APRON is online.
  const clrDelActive = !delOnline && !apronOnline;

  const nonClearedStrips = useNonClearedStrips();

  const bayStripMap = {
    "STARTUP":  { strips: startupStrips,                    targetBay: Bay.Cleared, descending: true },
    "PUSHBACK": { strips: pushStrips,                       targetBay: Bay.Push,      descending: true },
    "TWY-DEP":  { strips: twyDepDesc,                       targetBay: Bay.TaxiLwr,   descending: true },
    "RWY-DEP":  { strips: rwyDepStrips,                     targetBay: Bay.Depart,    descending: true },
    "AIRBORNE": { strips: airborneStrips,                   targetBay: Bay.Airborne,  descending: true },
    "DE-ICE":   { strips: deIceStrips,                      targetBay: Bay.DeIce,     descending: true },
    "STAND":    { strips: standStrips,                      targetBay: Bay.Stand,     descending: true },
    "FINAL":    { strips: finalStrips,                      targetBay: Bay.Final,     descending: true },
    "RWY-ARR":  { strips: rwyArrStrips,                     targetBay: Bay.RwyArr,    descending: true },
    "TWY-ARR":  { strips: twyArrStrips,                     targetBay: Bay.TwyArr,    descending: true },
    "CLRDEL":   { strips: clrDelActive ? nonClearedStrips : [], targetBay: Bay.NotCleared },
  };

  const transferRules = allBayTransferRules(Object.keys(bayStripMap));

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
      renderDragOverlay={(strip: AnyStrip) => {
        if (!isFlight(strip)) return <Strip strip={strip} width={CLX_CLEARED_STRIP_WIDTH} />;
        if (strip.bay === Bay.Cleared)   return <Strip strip={strip} status="PUSH" myPosition={myPosition} fullWidth />;
        if (strip.bay === Bay.Push)      return <Strip strip={strip} status="PUSH" myPosition={myPosition} fullWidth />;
        if (strip.bay === Bay.TaxiLwr)   return <div style={{ width: TWY_DEP_STRIP_WIDTH }}><Strip strip={strip} status="TWY-DEP" myPosition={myPosition} fullWidth /></div>;
        if (strip.bay === Bay.Depart)    return <div style={{ width: TWY_DEP_STRIP_WIDTH }}><Strip strip={strip} status="TWY-DEP" myPosition={myPosition} fullWidth /></div>;
        if (strip.bay === Bay.Airborne)  return <div style={{ width: TWY_DEP_STRIP_WIDTH }}><Strip strip={strip} status="TWY-DEP" myPosition={myPosition} fullWidth /></div>;
        if (strip.bay === Bay.DeIce)     return <Strip strip={strip} status="PUSH" myPosition={myPosition} />;
        if (strip.bay === Bay.Stand)     return <Strip strip={strip} status="ARR" myPosition={myPosition} />;
        if (strip.bay === Bay.Final)     return <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} />;
        if (strip.bay === Bay.RwyArr)    return <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} />;
        if (strip.bay === Bay.TwyArr)    return <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} />;
        if (strip.bay === Bay.NotCleared) return <Strip strip={strip} status="CLR" myPosition={myPosition} fullWidth />;
        return null;
      }}
    >
    <div className="bay-page-wrapper">

      {/* Column 1 (27%) – FINAL + RWY ARR + TWY ARR */}
      <div style={arrResize.columnStyle} className={COL_ARR}>
        <div className="bay-col-header justify-between">
          <span className={CLS_LABEL}>FINAL</span>
          <button className={CLS_BTN_ARR} onClick={() => setArrOpen(true)}>ARR</button>
        </div>
        <SortableBay
          strips={finalStrips}
          bayId="FINAL"
          isDragDisabled={(strip) => !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="h-[var(--bay-h-final)] bay-scroll-area-bottom"
        >
          {(strip) => (
            <Strip strip={strip} status="FINAL-ARR" selectable={false} myPosition={myPosition} />
          )}
        </SortableBay>

        <div className="bay-col-header bay-col-sep">
          <span className={CLS_LABEL}>RWY ARR</span>
          <BayResizeHandle {...arrResize.handleProps("final")} />
        </div>
        <SortableBay
          strips={rwyArrStrips}
          bayId="RWY-ARR"
          isDragDisabled={(strip) => !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="h-[var(--bay-h-rwyArr)] bay-scroll-area-dark"
        >
          {(strip) => (
            <Strip strip={strip} status="FINAL-ARR" selectable={false} myPosition={myPosition} />
          )}
        </SortableBay>

        <div className="bay-col-header bay-col-sep justify-between">
          <span className={CLS_LABEL}>TWY ARR</span>
          <BayResizeHandle {...arrResize.handleProps("rwyArr")} />
          <span className="flex gap-0.5">
            <MemAidButton bay={Bay.TwyArr} className={CLS_BTN_BLUE} />
            <LandButton bay={Bay.TwyArr} className={CLS_BTN_ORANGE} />
            <StartButton bay={Bay.TwyArr} className={CLS_BTN_ORANGE} />
            <CrossingButton bay={Bay.TwyArr} className={CLS_BTN_YELLOW} />
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
            <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} />
          )}
        </SortableBay>

        {arrOpen && (
          <StripListPopup
            title="ARR"
            strips={inboundStrips}
            sortModes={arrSortModes}
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
      <div style={depResize.columnStyle} className={COL_DEP}>
        <div className="bay-col-header">
          <span className={CLS_LABEL}>PUSHBACK</span>
        </div>
        <SortableBay
          strips={pushStrips}
          bayId="PUSHBACK"
          isDragDisabled={(strip) => !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="h-[var(--bay-h-push)] bay-scroll-area-bottom"
        >
          {(strip) => (
            <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} fullWidth />
          )}
        </SortableBay>

        <div className="bay-col-header bay-col-sep justify-between">
          <span className={CLS_LABEL}>TWY DEP</span>
          <BayResizeHandle {...depResize.handleProps("push")} />
          <span className="flex gap-0.5">
            <button className={CLS_BTN_NEW} onClick={() => setNewOpen(true)}>NEW</button>
            <MemAidButton bay={Bay.TaxiLwr} className={CLS_BTN_BLUE} />
            <LandButton bay={Bay.TaxiLwr} className={CLS_BTN_ORANGE} />
            <StartButton bay={Bay.TaxiLwr} className={CLS_BTN_ORANGE} />
            <CrossingButton bay={Bay.TaxiLwr} className={CLS_BTN_YELLOW} />
          </span>
        </div>
        <SortableBay
          strips={twyDepDesc}
          bayId="TWY-DEP"
          isDragDisabled={(strip) => !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="h-[var(--bay-h-twyDep)] bay-scroll-area-bottom"
        >
          {(strip) => (
            <Strip strip={strip} status="TWY-DEP" myPosition={myPosition} width={TWY_DEP_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>

        <div className="bay-col-header bay-col-sep">
          <span className={CLS_LABEL}>RWY DEP</span>
          <BayResizeHandle {...depResize.handleProps("twyDep")} />
        </div>
        <SortableBay
          strips={rwyDepStrips}
          bayId="RWY-DEP"
          isDragDisabled={(strip) => !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="h-[var(--bay-h-rwyDep)] bay-scroll-area-dark"
        >
          {(strip) => (
            <Strip strip={strip} status="TWY-DEP" myPosition={myPosition} width={TWY_DEP_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>

        <div className="bay-col-header bay-col-sep">
          <span className={CLS_LABEL}>AIRBORNE</span>
          <BayResizeHandle {...depResize.handleProps("rwyDep")} />
        </div>
        <SortableBay
          strips={airborneStrips}
          bayId="AIRBORNE"
          isDragDisabled={(strip) => !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="flex-1 bay-scroll-area-dark"
        >
          {(strip) => (
            <Strip strip={strip} status="TWY-DEP" myPosition={myPosition} width={TWY_DEP_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>
      </div>

      {/* Column 3 (25%) – STARTUP + DE-ICE A + MESSAGES */}
      <div style={clrDelResize.columnStyle} className={COL_CLRDEL}>
        <div className="bay-col-header justify-between">
          <span className={CLS_LABEL}>STARTUP</span>
          <button className={CLS_BTN_NEW} onClick={() => setNewOpen(true)}>NEW</button>
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
          <span className={CLS_LABEL}>DE-ICE A</span>
          <BayResizeHandle {...clrDelResize.handleProps("startup")} />
        </div>
        <SortableBay
          strips={deIceStrips}
          bayId="DE-ICE"
          isDragDisabled={(strip) => !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className="h-[var(--bay-h-deIce)] bay-scroll-area-bottom"
        >
          {(strip) => (
            <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className="bay-col-header-primary bay-col-sep justify-between">
          <span className="text-[#CECECE] font-bay tracking-[0.06em] [-webkit-text-stroke:0.5px_currentColor] text-[1.11375rem]">MESSAGES</span>
          <BayResizeHandle {...clrDelResize.handleProps("deIce")} />
          <span className="flex gap-0.5">
            <button className={CLS_BTN_NEW}>INFO</button>
            <button className={CLS_BTN_NEW}>MISC.</button>
            <button className={CLS_BTN_NEW}>EQUIP</button>
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
      <div style={standResize.columnStyle} className={COL_STAND}>
        <div className="bay-col-header justify-between">
          <span className={CLS_LABEL}>CLRDEL</span>
          <span className="flex gap-0.5">
            <button className={CLS_BTN_NEW} onClick={() => setNewOpen(true)}>NEW</button>
            <button className={CLS_BTN_PLANNED} onClick={() => setPlannedOpen(true)}>PLANNED</button>
          </span>
        </div>
        <SortableBay strips={clrDelActive ? nonClearedStrips : []} bayId="CLRDEL" standalone={false} className="h-[var(--bay-h-clrDel)] bay-scroll-area">
          {(strip) => <Strip strip={strip} status="CLR" selectable={false} myPosition={myPosition} fullWidth />}
        </SortableBay>

        <div className="bay-col-header bay-col-sep">
          <span className={CLS_LABEL}>STAND</span>
          <BayResizeHandle {...standResize.handleProps("clrDel")} />
        </div>
        <SortableBay
          strips={standStrips}
          bayId="STAND"
          isDragDisabled={(strip) => !!strip.owner && strip.owner !== myPosition}
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
