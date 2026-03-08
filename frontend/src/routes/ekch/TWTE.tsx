import { Strip } from "@/components/strip/Strip.tsx";
import { MemAidButton, CrossingButton, StartButton, LandButton } from "@/components/strip/TacticalButtons.tsx";
import { MessageStrip } from "@/components/strip/MessageStrip.tsx";
import { MessageComposeDialog } from "@/components/MessageComposeDialog.tsx";
import {
  useAirborneStrips,
  useDepartStrips,
  useDeIceStrips,
  useFinalStrips,
  usePushbackStrips,
  useStandStrips,
  useTaxiArrStrips,
  useTaxiDepStrips,
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
import { useWebSocketStore, useMyPosition, useLowerPositionOnline, useMessages } from "@/store/store-hooks.ts";
import { useRef, useEffect, useMemo, useState } from "react";
import { TWY_DEP_STRIP_WIDTH } from "@/components/strip/types";
import { StripListPopup, type SortMode } from "@/components/StripListPopup.tsx";

const scrollArea = "w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary";
const scrollAreaBottom = "w-full bg-[#555355] p-1 flex flex-col justify-end gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary";
const darkScrollAreaBottom = "w-full bg-[#212121] p-1 flex flex-col justify-end gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary";
const btn = "bg-[#646464] text-white font-bold text-sm px-3 border-2 border-white active:bg-[#424242]";
const btnOrange = "bg-[#DD6A12] text-white font-bold text-sm px-3 border-2 border-white active:bg-[#c45a0d]";
const btnBlue = "bg-[#004FD6] text-white font-bold text-sm px-3 border-2 border-white active:bg-[#003db0]";
const btnYellow = "bg-[#F3EA1F] text-black font-bold text-sm px-3 border-2 border-white active:bg-[#d4cb14]";

export default function TWTE() {
  const myPosition = useMyPosition();
  const messages   = useMessages();
  const [composeOpen, setComposeOpen] = useState(false);
  const [startupOpen, setStartupOpen] = useState(false);
  const [arrOpen, setArrOpen] = useState(false);
  const lowerPositionOnline = useLowerPositionOnline();

  const messagesEndRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  const finalStrips = useFinalStrips();
  const rwyArrStrips = useRwyArrStrips();
  const twyArrStrips = useTaxiArrStrips();

  const twyDepAll    = useTaxiDepStrips();
  const twyDepDesc   = useMemo(() => [...twyDepAll].reverse(), [twyDepAll]);
  const rwyDepAll    = useDepartStrips();
  const rwyDepDesc   = useMemo(() => [...rwyDepAll].reverse(), [rwyDepAll]);
  const airborneAll  = useAirborneStrips();
  const airborneDesc = useMemo(() => [...airborneAll].reverse(), [airborneAll]);
  const standStrips  = useStandStrips();
  const pushStrips   = usePushbackStrips();
  const deIceStrips  = useDeIceStrips();
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

  const ALL_ACTIVE = ["FINAL", "RWY-ARR", "TWY-ARR", "TWY-DEP", "RWY-DEP", "AIRBORNE", "STAND", "PUSHBACK", "DE-ICE"] as const;

  const bayStripMap = {
    "FINAL":    { strips: finalStrips, targetBay: Bay.Final, descending: true },
    "RWY-ARR":  { strips: rwyArrStrips,  targetBay: Bay.RwyArr, descending: true },
    "TWY-ARR":  { strips: twyArrStrips,   targetBay: Bay.TwyArr, descending: true },
    "TWY-DEP":  { strips: twyDepDesc,     targetBay: Bay.Taxi,     descending: true },
    "RWY-DEP":  { strips: rwyDepDesc,     targetBay: Bay.Depart,   descending: true },
    "AIRBORNE": { strips: airborneDesc,   targetBay: Bay.Airborne, descending: true },
    "STAND":    { strips: standStrips,    targetBay: Bay.Stand },
    "PUSHBACK": { strips: pushStrips,     targetBay: Bay.Push },
    "DE-ICE":   { strips: deIceStrips,    targetBay: Bay.DeIce },
  };

  const transferRules: Record<string, string[]> = {
    "FINAL":    ["RWY-ARR", "TWY-ARR"],
    "RWY-ARR":  ALL_ACTIVE.filter(b => b !== "RWY-ARR"),
    "TWY-ARR":  ALL_ACTIVE.filter(b => b !== "TWY-ARR"),
    "TWY-DEP":  ALL_ACTIVE.filter(b => b !== "TWY-DEP"),
    "RWY-DEP":  ALL_ACTIVE.filter(b => b !== "RWY-DEP"),
    "AIRBORNE": ALL_ACTIVE.filter(b => b !== "AIRBORNE"),
    "STAND":    ALL_ACTIVE.filter(b => b !== "STAND"),
    "PUSHBACK": ["TWY-DEP", "DE-ICE", "TWY-ARR"],
    "DE-ICE":   ["PUSHBACK", "TWY-DEP"],
  };

  const statusForBay: Record<string, StripStatus> = {
    "FINAL": "FINAL-ARR", "RWY-ARR": "FINAL-ARR",
    "TWY-ARR": "FINAL-ARR", "TWY-DEP": "TWY-DEP",
    "RWY-DEP": "TWY-DEP", "AIRBORNE": "TWY-DEP",
    "STAND": "CLROK", "PUSHBACK": "PUSH", "DE-ICE": "PUSH",
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
    <div className="bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center gap-2">

      {/* Column 1 – FINAL + RWY ARR + TWY ARR */}
      <div className="w-[24.5%] h-full bg-[#555355] flex flex-col">
        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 justify-between">
          <span className="text-white font-bold text-lg">FINAL</span>
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
            <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} />
          )}
        </SortableBay>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 border-t-4 border-[#A9A9A9] justify-between">
          <span className="text-white font-bold text-lg">RWY ARR</span>
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
            <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} />
          )}
        </SortableBay>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 border-t-4 border-[#A9A9A9] justify-between">
          <span className="text-white font-bold text-lg">TWY ARR</span>
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
          <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} />
          )}
        </SortableBay>
      </div>

      {/* Column 2 – TWY DEP + RWY DEP + AIRBORNE */}
      <div className="w-[28.5%] h-full bg-[#555355] flex flex-col">
        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 justify-between">
          <span className="text-white font-bold text-lg">TWY DEP</span>
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
            <Strip strip={strip} status="TWY-DEP" myPosition={myPosition} width={TWY_DEP_STRIP_WIDTH} />
          )}
        </SortableBay>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 justify-between border-t-4 border-[#A9A9A9]">
          <span className="text-white font-bold text-lg">RWY DEP</span>
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
            <Strip strip={strip} status="TWY-DEP" myPosition={myPosition} width={TWY_DEP_STRIP_WIDTH} />
          )}
        </SortableBay>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 border-t-4 border-[#A9A9A9]">
          <span className="text-white font-bold text-lg">AIRBORNE</span>
        </div>
        <SortableBay
          strips={airborneDesc}
          bayId="AIRBORNE"
          standalone={false}
          className={`flex-1 ${scrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="TWY-DEP" myPosition={myPosition} width={TWY_DEP_STRIP_WIDTH} />
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
      <div className="w-[24.5%] h-full bg-[#555355] flex flex-col">
        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 justify-between">
          <span className="text-white font-bold text-lg">CONTROLZONE</span>
          <span className="flex gap-1">
            <button className={btn}>NEW</button>
            <button className={btn}>FIND</button>
          </span>
        </div>
        {/* VFR strips – bay TBD with backend */}
        <div className={`h-[35%] ${scrollArea}`} />

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 border-t-4 border-[#A9A9A9]">
          <span className="text-white font-bold text-lg">PUSHBACK</span>
        </div>
        <SortableBay
          strips={pushStrips}
          bayId="PUSHBACK"
          standalone={false}
          className={`h-[35%] ${scrollArea}`}
        >
          {(strip) => <Strip strip={strip} status="PUSH" myPosition={myPosition} />}
        </SortableBay>

        <div className="bg-primary h-10 flex items-center px-2 shrink-0 justify-between border-t-4 border-[#A9A9A9]">
          <span className="text-white font-bold text-lg">MESSAGES</span>
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
      <div className="w-[20.5%] h-full bg-[#555355] flex flex-col">
        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 justify-between">
          <span className="text-white font-bold text-lg">CLRDEL</span>
          <span className="flex gap-1">
            <button className={btn}>NEW</button>
            <button className={btn}>PLANNED</button>
          </span>
        </div>
        <DropIndicatorBay bayId="CLRDEL" className={`h-[45%] ${scrollArea}`}>
          {nonClearedStrips.map(s => (
            <Strip key={s.callsign} strip={s} status={lowerPositionOnline ? "CLX-HALF" : "CLR"} fullWidth={true} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 justify-between border-t-4 border-[#A9A9A9]">
          <span className="text-white font-bold text-lg">DE-ICE A</span>
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
          {(strip) => <Strip strip={strip} status="PUSH" myPosition={myPosition} />}
        </SortableBay>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 border-t-4 border-[#A9A9A9]">
          <span className="text-white font-bold text-lg">STAND</span>
        </div>
        <SortableBay
          strips={standStrips}
          bayId="STAND"
          standalone={false}
          className={`flex-1 ${scrollArea}`}
        >
          {(strip) => <Strip strip={strip} status="CLROK" myPosition={myPosition} />}
        </SortableBay>
      </div>

    </div>
    </ViewDndContext>
  );
}
