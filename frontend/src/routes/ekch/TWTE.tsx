import { Strip } from "@/components/strip/Strip.tsx";
import { MemAidButton, CrossingButton, StartButton, LandButton } from "@/components/strip/TacticalButtons.tsx";
import { MessageStrip } from "@/components/strip/MessageStrip.tsx";
import { MessageComposeDialog } from "@/components/MessageComposeDialog.tsx";
import {
  useAirborneStrips,
  useDepartStrips,
  useDeIceStrips,
  useControlzoneStrips,
  useFinalStrips,
  usePushbackStrips,
  useStandStrips,
  useTaxiArrStrips,
  useTaxiDepLwrStrips,
  useNonClearedStrips,
  useClearedStrips,
  isFlight,
  useInboundStrips,
  useRwyArrStrips,
} from "@/store/airports/ekch.ts";
import type { AnyStrip, StripRef, FrontendStrip } from "@/api/models.ts";
import { Bay } from "@/api/models.ts";
import type { StripStatus } from "@/components/strip/types.ts";
import { SortableBay, DropIndicatorBay } from "@/components/bays/SortableBay.tsx";
import { ViewDndContext } from "@/components/bays/ViewDndContext.tsx";
import { useWebSocketStore, useMyPosition, useLowerPositionOnline, useCtwrOnline, useMessages } from "@/store/store-hooks.ts";
import { useRef, useEffect, useMemo, useState } from "react";
import { TWY_DEP_STRIP_WIDTH } from "@/components/strip/types";
import { StripListPopup, type SortMode } from "@/components/StripListPopup.tsx";
import { CLS_BTN, CLS_BTN_ORANGE, CLS_BTN_BLUE, CLS_BTN_YELLOW, CLS_SCROLLBAR, CLS_COL } from "@/components/strip/shared";

// Column widths
const W_COL_ARR      = "w-[24.5%]";
const W_COL_DEP      = "w-[28.5%]";
const W_COL_CENTER   = "w-[24.5%]";
const W_COL_RIGHT    = "w-[20.5%]";

// Header class strings
const lockedHeader = "bg-[#393939] h-10 flex items-center px-2 shrink-0";
const lockedLabel  = "text-white font-bold text-lg";
const activeHeader = "bg-[#b3b3b3] h-10 flex items-center px-2 shrink-0";
const activeLabel  = "text-[#393939] font-bold text-lg";

// Section separator (grey border between sub-sections within a column)
const colSep      = "border-t-4 border-[#A9A9A9]";
const pageWrapper = "bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center gap-2";

const scrollArea           = `w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto ${CLS_SCROLLBAR}`;
const scrollAreaBottom     = `w-full bg-[#555355] p-1 flex flex-col justify-end gap-px overflow-y-auto ${CLS_SCROLLBAR}`;
const darkScrollAreaBottom = `w-full bg-[#212121] p-1 flex flex-col justify-end gap-px overflow-y-auto ${CLS_SCROLLBAR}`;
const btn       = CLS_BTN;
const btnOrange = CLS_BTN_ORANGE;
const btnBlue   = CLS_BTN_BLUE;
const btnYellow = CLS_BTN_YELLOW;

export default function TWTE() {
  const myPosition = useMyPosition();
  const messages   = useMessages();
  const [composeOpen, setComposeOpen] = useState(false);
  const [startupOpen, setStartupOpen] = useState(false);
  const [arrOpen, setArrOpen] = useState(false);
  const lowerPositionOnline = useLowerPositionOnline();
  const ctwrOnline = useCtwrOnline();
  // TE/TW is responsible for clearances only when no lower position AND no CTWR is online.
  const clrDelActive = !lowerPositionOnline && !ctwrOnline;

  const messagesEndRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  const finalStrips = useFinalStrips();
  const rwyArrStrips = useRwyArrStrips();
  const twyArrStrips = useTaxiArrStrips();

  const twyDepAll    = useTaxiDepLwrStrips();
  const twyDepDesc   = useMemo(() => [...twyDepAll].reverse(), [twyDepAll]);
  const rwyDepAll    = useDepartStrips();
  const rwyDepDesc   = useMemo(() => [...rwyDepAll].reverse(), [rwyDepAll]);
  const airborneAll  = useAirborneStrips();
  const airborneDesc = useMemo(() => [...airborneAll].reverse(), [airborneAll]);
  const standStrips  = useStandStrips();
  const pushStrips   = usePushbackStrips();
  const deIceStrips  = useDeIceStrips();
  const controlzoneStrips = useControlzoneStrips();
  const nonClearedStrips = useNonClearedStrips();
  const clearedStrips    = useClearedStrips();
  const inboundStrips    = useInboundStrips();

  const updateOrder       = useWebSocketStore(state => state.updateOrder);
  const move              = useWebSocketStore(state => state.move);
  const moveTacticalStrip = useWebSocketStore(state => state.moveTacticalStrip);
  const assumeStrip       = useWebSocketStore(state => state.assumeStrip);

  const startupSortModes: SortMode<FrontendStrip>[] = [
    { key: "EOBT",     label: "EOBT",     compareFn: (a, b) => a.eobt.localeCompare(b.eobt) },
    { key: "CALLSIGN", label: "CALLSIGN", compareFn: (a, b) => a.callsign.localeCompare(b.callsign) },
    { key: "ADES",     label: "ADES",     compareFn: (a, b) => a.destination.localeCompare(b.destination) },
  ];

  const arrSortModes: SortMode<FrontendStrip>[] = [
    { key: "ETA",      label: "ETA",      compareFn: (a, b) => a.eldt.localeCompare(b.eldt) },
    { key: "CALLSIGN", label: "CALLSIGN", compareFn: (a, b) => a.callsign.localeCompare(b.callsign) },
    { key: "ADEP",     label: "ADEP",     compareFn: (a, b) => a.origin.localeCompare(b.origin) },
  ];

  const ALL_ACTIVE = ["FINAL", "RWY-ARR", "TWY-ARR", "TWY-DEP", "RWY-DEP", "AIRBORNE", "STAND", "PUSHBACK", "DE-ICE", "CONTROLZONE"] as const;

  const bayStripMap = {
    "FINAL":       { strips: finalStrips,       targetBay: Bay.Final,       descending: true },
    "RWY-ARR":     { strips: rwyArrStrips,       targetBay: Bay.RwyArr,      descending: true },
    "TWY-ARR":     { strips: twyArrStrips,       targetBay: Bay.TwyArr,      descending: true },
    "TWY-DEP":     { strips: twyDepDesc,         targetBay: Bay.TaxiLwr,     descending: true },
    "RWY-DEP":     { strips: rwyDepDesc,         targetBay: Bay.Depart,      descending: true },
    "AIRBORNE":    { strips: airborneDesc,       targetBay: Bay.Airborne,    descending: true },
    "STAND":       { strips: standStrips,        targetBay: Bay.Stand },
    "PUSHBACK":    { strips: pushStrips,         targetBay: Bay.Push },
    "DE-ICE":      { strips: deIceStrips,        targetBay: Bay.DeIce },
    "CONTROLZONE": { strips: controlzoneStrips,  targetBay: Bay.Controlzone },
  };

  const transferRules: Record<string, string[]> = {
    "FINAL":       ["RWY-ARR", "TWY-ARR"],
    "RWY-ARR":     ALL_ACTIVE.filter(b => b !== "RWY-ARR"),
    "TWY-ARR":     ALL_ACTIVE.filter(b => b !== "TWY-ARR"),
    "TWY-DEP":     ALL_ACTIVE.filter(b => b !== "TWY-DEP"),
    "RWY-DEP":     ALL_ACTIVE.filter(b => b !== "RWY-DEP"),
    "AIRBORNE":    ALL_ACTIVE.filter(b => b !== "AIRBORNE"),
    "STAND":       ALL_ACTIVE.filter(b => b !== "STAND"),
    "PUSHBACK":    ["TWY-DEP", "DE-ICE", "TWY-ARR"],
    "DE-ICE":      ["PUSHBACK", "TWY-DEP"],
    "CONTROLZONE": [],
  };

  const statusForBay: Record<string, StripStatus> = {
    "FINAL": "FINAL-ARR", "RWY-ARR": "FINAL-ARR",
    "TWY-ARR": "FINAL-ARR", "TWY-DEP": "TWY-DEP",
    "RWY-DEP": "TWY-DEP", "AIRBORNE": "TWY-DEP",
    "STAND": "CLROK", "PUSHBACK": "PUSH", "DE-ICE": "PUSH",
    "CONTROLZONE": "CLR",
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
        if (!isFlight(strip)) return <Strip strip={strip} width={TWY_DEP_STRIP_WIDTH} />;
        const bayEntry = Object.entries(bayStripMap).find(([, config]) =>
          config.strips.some(s => isFlight(s) && s.callsign === strip.callsign)
        );
        if (!bayEntry) return null;
        const [bayId] = bayEntry;
        return (
          <Strip
            strip={strip}
            status={statusForBay[bayId]}
            myPosition={myPosition}
          />
        );
      }}
    >
    <div className={pageWrapper}>

      {/* Column 1 – FINAL + RWY ARR + TWY ARR */}
      <div className={`${W_COL_ARR} ${CLS_COL}`}>
        <div className={`${lockedHeader} justify-between`}>
          <span className={lockedLabel}>FINAL</span>
          <span className="flex gap-1">
            <button className={btn} onClick={() => setArrOpen(true)}>ARR</button>
          </span>
        </div>
        <SortableBay
          strips={finalStrips}
          bayId="FINAL"
          standalone={false}
          className={`h-[35%] ${scrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className={`${lockedHeader} ${colSep} justify-between`}>
          <span className={lockedLabel}>RWY ARR</span>
          <span className="flex gap-1">
            <button className={btn}>MISSED APP</button>
            <LandButton bay={Bay.Final} className={btnOrange} />
            <StartButton bay={Bay.Final} className={btnOrange} />
            <CrossingButton bay={Bay.Final} className={btnYellow} />
          </span>
        </div>
        <SortableBay
          strips={rwyArrStrips}
          bayId="RWY-ARR"
          standalone={false}
          className={`h-[20%] ${darkScrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className={`${lockedHeader} ${colSep} justify-between`}>
          <span className={lockedLabel}>TWY ARR</span>
          <span className="flex gap-1">
            <MemAidButton bay={Bay.Taxi} className={btnBlue} />
            <LandButton bay={Bay.Taxi} className={btnOrange} />
            <StartButton bay={Bay.Taxi} className={btnOrange} />
            <CrossingButton bay={Bay.Taxi} className={btnYellow} />
          </span>
        </div>
        <SortableBay
          strips={twyArrStrips}
          bayId="TWY-ARR"
          standalone={false}
          className={`flex-1 ${scrollAreaBottom}`}
        >
          {(strip) => (
          <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>
      </div>

      {/* Column 2 – TWY DEP + RWY DEP + AIRBORNE */}
      <div className={`${W_COL_DEP} ${CLS_COL}`}>
        <div className={`${lockedHeader} justify-between`}>
          <span className={lockedLabel}>TWY DEP</span>
          <span className="flex gap-1">
            <button className={btn} onClick={() => setStartupOpen(true)}>STARTUP</button>
            <MemAidButton bay={Bay.Taxi} className={btnBlue} />
            <LandButton bay={Bay.Taxi} className={btnOrange} />
            <StartButton bay={Bay.Taxi} className={btnOrange} />
            <CrossingButton bay={Bay.Taxi} className={btnYellow} />
          </span>
        </div>
        <SortableBay
          strips={twyDepDesc}
          bayId="TWY-DEP"
          standalone={false}
          className={`h-[35%] ${scrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="TWY-DEP" myPosition={myPosition} width={TWY_DEP_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>

        <div className={`${lockedHeader} ${colSep} justify-between`}>
          <span className={lockedLabel}>RWY DEP</span>
          <span className="flex gap-1">
            <LandButton bay={Bay.Depart} className={btnOrange} />
            <StartButton bay={Bay.Depart} className={btnOrange} />
            <CrossingButton bay={Bay.Depart} className={btnYellow} />
          </span>
        </div>
        <SortableBay
          strips={rwyDepDesc}
          bayId="RWY-DEP"
          standalone={false}
          className={`h-[20%] ${darkScrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="TWY-DEP" myPosition={myPosition} width={TWY_DEP_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>

        <div className={`${lockedHeader} ${colSep}`}>
          <span className={lockedLabel}>AIRBORNE</span>
        </div>
        <SortableBay
          strips={airborneDesc}
          bayId="AIRBORNE"
          standalone={false}
          className={`flex-1 ${scrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="TWY-DEP" myPosition={myPosition} width={TWY_DEP_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>

        {startupOpen && (
          <StripListPopup
            title="STARTUP"
            strips={clearedStrips}
            sortModes={startupSortModes}
            onRowClick={(strip) => {
              move(strip.callsign, Bay.Push);
              assumeStrip(strip.callsign);
              setStartupOpen(false);
            }}
            onDismiss={() => setStartupOpen(false)}
            myPosition={myPosition}
          />
        )}
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

      {/* Column 3 – CONTROLZONE + PUSHBACK + MESSAGES */}
      <div className={`${W_COL_CENTER} ${CLS_COL}`}>
        <div className={`${lockedHeader} justify-between`}>
          <span className={lockedLabel}>CONTROLZONE</span>
          <span className="flex gap-1">
            <button className={btn}>NEW</button>
            <button className={btn}>FIND</button>
          </span>
        </div>
        <SortableBay
          strips={controlzoneStrips}
          bayId="CONTROLZONE"
          standalone={false}
          className={`h-[35%] ${scrollArea}`}
        >
          {(strip) => <Strip strip={strip} status="CLR" myPosition={myPosition} selectable={true} />}
        </SortableBay>

        <div className={`${lockedHeader} ${colSep}`}>
          <span className={lockedLabel}>PUSHBACK</span>
        </div>
        <SortableBay
          strips={pushStrips}
          bayId="PUSHBACK"
          standalone={false}
          className={`h-[35%] ${scrollArea}`}
        >
          {(strip) => <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} />}
        </SortableBay>

        <div className={`bg-primary h-10 flex items-center px-2 shrink-0 justify-between ${colSep}`}>
          <span className={lockedLabel}>MESSAGES</span>
          <span className="flex gap-1">
            <button className={btn} onClick={() => setComposeOpen(true)}>INFO</button>
            <button className={btn} onClick={() => setComposeOpen(true)}>MISC.</button>
            <button className={btn} onClick={() => setComposeOpen(true)}>EQUIP</button>
          </span>
        </div>
        <div className={`flex-1 ${scrollArea}`}>
          {messages.map(msg => (
            <MessageStrip key={msg.id} msg={msg} />
          ))}
          <div ref={messagesEndRef} />
        </div>
        <MessageComposeDialog open={composeOpen} onClose={() => setComposeOpen(false)} />
      </div>

      {/* Column 4 – CLRDEL + DE-ICE A + STAND */}
      <div className={`${W_COL_RIGHT} ${CLS_COL}`}>
        <div className={`${clrDelActive ? activeHeader : lockedHeader} justify-between`}>
          <span className={clrDelActive ? activeLabel : lockedLabel}>CLRDEL</span>
          <span className="flex gap-1">
            <button className={btn}>NEW</button>
            <button className={btn}>PLANNED</button>
          </span>
        </div>
        <DropIndicatorBay bayId="CLRDEL" className={`h-[45%] ${scrollArea}`}>
          {nonClearedStrips.map(s => (
            <Strip key={s.callsign} strip={s} status={clrDelActive ? "CLR" : "CLX-HALF"} fullWidth={true} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className={`${lockedHeader} ${colSep} justify-between`}>
          <span className={lockedLabel}>DE-ICE A</span>
          <span className="flex gap-1">
            <button className={btn}>DI A</button>
            <button className={btn}>DI B</button>
            <button className={btn}>DI V</button>
          </span>
        </div>
        <SortableBay
          strips={deIceStrips}
          bayId="DE-ICE"
          standalone={false}
          className={`h-[25%] ${scrollArea}`}
        >
          {(strip) => <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} />}
        </SortableBay>

        <div className={`${lockedHeader} ${colSep}`}>
          <span className={lockedLabel}>STAND</span>
        </div>
        <SortableBay
          strips={standStrips}
          bayId="STAND"
          standalone={false}
          className={`flex-1 ${scrollArea}`}
        >
          {(strip) => <Strip strip={strip} status="CLROK" myPosition={myPosition} selectable={true} />}
        </SortableBay>
      </div>

    </div>
    </ViewDndContext>
  );
}
