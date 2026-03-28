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
import { useWebSocketStore, useMyPosition, useLowerPositionOnline, useCtwrOnline, useMessages, useSelectedCallsign, useSelectStrip, useAirport } from "@/store/store-hooks.ts";
import { useRef, useEffect, useState, useCallback } from "react";
import missedApproachSound from "@/assets/missed_approach.mp3";
import { isAudioMuted } from "@/lib/audio-settings";
import { TWY_DEP_STRIP_WIDTH } from "@/components/strip/types";
import { StripListPopup, type SortMode } from "@/components/StripListPopup.tsx";
import { CLS_BTN, CLS_BTN_ORANGE, CLS_BTN_BLUE, CLS_BTN_YELLOW, CLS_SCROLLBAR, CLS_COL, CLS_HEADER_SHADOW } from "@/components/strip/shared";
import { NewIfrDialog } from "@/components/strip/NewIfrDialog";
import { NewVfrDialog } from "@/components/strip/NewVfrDialog";
import { PlannedDialog } from "@/components/strip/PlannedDialog";
import { FindDialog } from "@/components/strip/FindDialog";

// Column widths
const W_COL_ARR      = "w-1/4";
const W_COL_DEP      = "w-[29%]";
const W_COL_CENTER   = "w-1/4";
const W_COL_RIGHT    = "w-[21%]";

// Header class strings
const header = `bg-[#393939] h-10 flex items-center px-2 shrink-0 ${CLS_HEADER_SHADOW}`;
const label  = "text-white font-bold text-lg";

// Section separator (grey border between sub-sections within a column)
const colSep      = "border-t-[6px] border-bay-border";
const pageWrapper = "bg-bay-border w-screen h-[95.28vh] flex divide-x-[6px] divide-bay-border border-x-2 border-t-2 border-bay-border";

const scrollArea           = `w-full bg-[#555355] shadow-[inset_2px_2px_4px_rgba(0,0,0,0.55),inset_-1px_-1px_2px_rgba(255,255,255,0.07)] p-0.5 flex flex-col gap-px overflow-y-auto ${CLS_SCROLLBAR}`;
const scrollAreaBottom     = `w-full bg-[#555355] shadow-[inset_2px_2px_4px_rgba(0,0,0,0.55),inset_-1px_-1px_2px_rgba(255,255,255,0.07)] p-0.5 flex flex-col justify-end gap-px overflow-y-auto ${CLS_SCROLLBAR}`;
const darkScrollAreaBottom = `w-full bg-[#212121] shadow-[inset_3px_3px_7px_rgba(0,0,0,0.85),inset_-1px_-1px_3px_rgba(255,255,255,0.05)] p-0.5 flex flex-col justify-end gap-px overflow-y-auto ${CLS_SCROLLBAR}`;
const btn       = CLS_BTN;
const btnOrange = CLS_BTN_ORANGE;
const btnBlue   = CLS_BTN_BLUE;
const btnYellow = CLS_BTN_YELLOW;

export default function TWTE() {
  const myPosition = useMyPosition();
  const airport    = useAirport();
  const messages   = useMessages();
  const [composeOpen, setComposeOpen] = useState(false);
  const [startupOpen, setStartupOpen] = useState(false);
  const [arrOpen, setArrOpen] = useState(false);
  const [newIfrOpen, setNewIfrOpen] = useState(false);
  const [plannedOpen, setPlannedOpen] = useState(false);
  const [newVfrOpen, setNewVfrOpen] = useState(false);
  const [findOpen, setFindOpen] = useState(false);
  const lowerPositionOnline = useLowerPositionOnline();
  const ctwrOnline = useCtwrOnline();
  // TE/TW is responsible for clearances only when no lower position AND no CTWR is online.
  const clrDelActive = !lowerPositionOnline && !ctwrOnline;

  const messagesEndRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  const finalStrips   = useFinalStrips().sort((a, b) => b.sequence - a.sequence);
  const rwyArrStrips  = useRwyArrStrips().sort((a, b) => b.sequence - a.sequence);
  const twyArrStrips  = useTaxiArrStrips().sort((a, b) => b.sequence - a.sequence);

  const twyDepDesc    = useTaxiDepLwrStrips().sort((a, b) => b.sequence - a.sequence);
  const rwyDepDesc    = useDepartStrips().sort((a, b) => b.sequence - a.sequence);
  const airborneDesc  = useAirborneStrips().sort((a, b) => b.sequence - a.sequence);
  const standStrips   = useStandStrips().sort((a, b) => b.sequence - a.sequence);
  const pushStrips    = usePushbackStrips().sort((a, b) => b.sequence - a.sequence);
  const deIceStrips   = useDeIceStrips().sort((a, b) => b.sequence - a.sequence);
  const controlzoneStrips = useControlzoneStrips().sort((a, b) => b.sequence - a.sequence);
  const nonClearedStrips = useNonClearedStrips();
  const clearedStrips    = useClearedStrips();
  const inboundStrips    = useInboundStrips();

  const updateOrder       = useWebSocketStore(state => state.updateOrder);
  const move              = useWebSocketStore(state => state.move);
  const moveTacticalStrip = useWebSocketStore(state => state.moveTacticalStrip);
  const assumeStrip       = useWebSocketStore(state => state.assumeStrip);
  const missedApproach    = useWebSocketStore(state => state.missedApproach);

  const selectedCallsign  = useSelectedCallsign();
  const selectStrip       = useSelectStrip();

  // A strip is eligible for MISSED APP if it's in FINAL or RWY ARR and owned by us.
  const selectedStrip = selectedCallsign
    ? [...finalStrips, ...rwyArrStrips].find(s => isFlight(s) && s.callsign === selectedCallsign) as FrontendStrip | undefined
    : undefined;
  const canMissedApproach = !!selectedStrip && selectedStrip.owner === myPosition;

  const handleMissedApproach = useCallback(() => {
    if (!selectedStrip || !canMissedApproach) return;
    if (!isAudioMuted()) new Audio(missedApproachSound).play().catch(() => {});
    missedApproach(selectedStrip.callsign);
    selectStrip(null);
  }, [selectedStrip, canMissedApproach, missedApproach, selectStrip]);

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
    "STAND":       { strips: standStrips,        targetBay: Bay.Stand,        descending: true },
    "PUSHBACK":    { strips: pushStrips,         targetBay: Bay.Push,         descending: true },
    "DE-ICE":      { strips: deIceStrips,        targetBay: Bay.DeIce,        descending: true },
    "CONTROLZONE": { strips: controlzoneStrips,  targetBay: Bay.Controlzone,  descending: true },
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
    "STAND": "ARR", "PUSHBACK": "PUSH", "DE-ICE": "PUSH",
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
        <div className={`${header} justify-between`}>
          <span className={label}>FINAL</span>
          <span className="flex gap-1">
            <button className={btn} onClick={() => setArrOpen(true)}>ARR</button>
          </span>
        </div>
        <SortableBay
          strips={finalStrips}
          bayId="FINAL"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[35%] ${scrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className={`${header} ${colSep} justify-between`}>
          <span className={label}>RWY ARR</span>
          <span className="flex gap-1">
            <button
              className={canMissedApproach ? btn : `${btn} opacity-40 cursor-not-allowed`}
              onClick={handleMissedApproach}
              disabled={!canMissedApproach}
            >MISSED APP</button>
            <LandButton bay={Bay.RwyArr} className={btnOrange} />
            <StartButton bay={Bay.RwyArr} className={btnOrange} />
            <CrossingButton bay={Bay.RwyArr} className={btnYellow} />
          </span>
        </div>
        <SortableBay
          strips={rwyArrStrips}
          bayId="RWY-ARR"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[20%] ${darkScrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className={`${header} ${colSep} justify-between`}>
          <span className={label}>TWY ARR</span>
          <span className="flex gap-1">
            <MemAidButton bay={Bay.TwyArr} className={btnBlue} />
            <LandButton bay={Bay.TwyArr} className={btnOrange} />
            <StartButton bay={Bay.TwyArr} className={btnOrange} />
            <CrossingButton bay={Bay.TwyArr} className={btnYellow} />
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
          <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>
      </div>

      {/* Column 2 – TWY DEP + RWY DEP + AIRBORNE */}
      <div className={`${W_COL_DEP} ${CLS_COL}`}>
        <div className={`${header} justify-between`}>
          <span className={label}>TWY DEP</span>
          <span className="flex gap-1">
            <button className={btn} onClick={() => setStartupOpen(true)}>STARTUP</button>
            <MemAidButton bay={Bay.TaxiLwr} className={btnBlue} />
            <LandButton bay={Bay.TaxiLwr} className={btnOrange} />
            <StartButton bay={Bay.TaxiLwr} className={btnOrange} />
            <CrossingButton bay={Bay.TaxiLwr} className={btnYellow} />
          </span>
        </div>
        <SortableBay
          strips={twyDepDesc}
          bayId="TWY-DEP"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[35%] ${scrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="TWY-DEP" myPosition={myPosition} width={TWY_DEP_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>

        <div className={`${header} ${colSep} justify-between`}>
          <span className={label}>RWY DEP</span>
          <span className="flex gap-1">
            <LandButton bay={Bay.Depart} className={btnOrange} />
            <StartButton bay={Bay.Depart} className={btnOrange} />
            <CrossingButton bay={Bay.Depart} className={btnYellow} />
          </span>
        </div>
        <SortableBay
          strips={rwyDepDesc}
          bayId="RWY-DEP"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[20%] ${darkScrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="TWY-DEP" myPosition={myPosition} width={TWY_DEP_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>

        <div className={`${header} ${colSep}`}>
          <span className={label}>AIRBORNE</span>
        </div>
        <SortableBay
          strips={airborneDesc}
          bayId="AIRBORNE"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`flex-1 ${scrollAreaBottom}`}
        >
          {(strip) => {
            const isArrival = isFlight(strip) && strip.destination === airport;
            return isArrival
              ? <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} selectable={true} />
              : <Strip strip={strip} status="TWY-DEP" myPosition={myPosition} width={TWY_DEP_STRIP_WIDTH} selectable={true} />;
          }}
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
        <div className={`${header} justify-between`}>
          <span className={label}>CONTROLZONE</span>
          <span className="flex gap-1">
            <button className={btn} onClick={() => setNewVfrOpen(true)}>NEW</button>
            <button className={btn} onClick={() => setFindOpen(true)}>FIND</button>
          </span>
        </div>
        <SortableBay
          strips={controlzoneStrips}
          bayId="CONTROLZONE"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[35%] ${scrollAreaBottom}`}
        >
          {(strip) => <Strip strip={strip} status="CLR" myPosition={myPosition} selectable={true} />}
        </SortableBay>

        <div className={`${header} ${colSep}`}>
          <span className={label}>PUSHBACK</span>
        </div>
        <SortableBay
          strips={pushStrips}
          bayId="PUSHBACK"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[35%] ${scrollAreaBottom}`}
        >
          {(strip) => <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} />}
        </SortableBay>

        <div className={`bg-primary h-10 flex items-center px-2 shrink-0 justify-between ${colSep}`}>
          <span className={label}>MESSAGES</span>
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
        <div className={`${header} justify-between`}>
          <span className={label}>CLRDEL</span>
          <span className="flex gap-1">
            <button className={btn} onClick={() => setNewIfrOpen(true)}>NEW</button>
            <button className={btn} onClick={() => setPlannedOpen(true)}>PLANNED</button>
          </span>
        </div>
        <DropIndicatorBay bayId="CLRDEL" className={`h-[45%] ${scrollArea}`}>
          {nonClearedStrips.map(s => (
            <Strip key={s.callsign} strip={s} status={clrDelActive ? "CLR" : "CLX-HALF"} fullWidth={true} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className={`${header} ${colSep} justify-between`}>
          <span className={label}>DE-ICE A</span>
          <span className="flex gap-1">
            <button className={btn}>DI A</button>
            <button className={btn}>DI B</button>
            <button className={btn}>DI V</button>
          </span>
        </div>
        <SortableBay
          strips={deIceStrips}
          bayId="DE-ICE"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[25%] ${scrollAreaBottom}`}
        >
          {(strip) => <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} />}
        </SortableBay>

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
          {(strip) => <Strip strip={strip} status="ARR" myPosition={myPosition} selectable={true} />}
        </SortableBay>
      </div>

    </div>
    <NewIfrDialog open={newIfrOpen} onOpenChange={setNewIfrOpen} />
    <PlannedDialog open={plannedOpen} onOpenChange={setPlannedOpen} />
    <NewVfrDialog open={newVfrOpen} onOpenChange={setNewVfrOpen} />
    <FindDialog open={findOpen} onOpenChange={setFindOpen} />
    </ViewDndContext>
  );
}
