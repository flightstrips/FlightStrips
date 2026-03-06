import { Strip } from "@/components/strip/Strip.tsx";
import { MemAidButton, CrossingButton, StartButton, LandButton } from "@/components/strip/TacticalButtons.tsx";
import { Message } from "@/components/Message.tsx";
import {
  useActiveMessages,
  useAirborneStrips,
  useDepartStrips,
  useDeIceStrips,
  useFinalStrips,
  usePushbackStrips,
  useStandStrips,
  useTaxiArrStrips,
  useTaxiDepStrips,
  useNonClearedStrips,
  isFlight,
} from "@/store/airports/ekch.ts";
import type { AnyStrip, StripRef } from "@/api/models.ts";
import { Bay } from "@/api/models.ts";
import type { HalfStripVariant, StripStatus } from "@/components/strip/types.ts";
import { SortableBay, DropIndicatorBay } from "@/components/bays/SortableBay.tsx";
import { ViewDndContext } from "@/components/bays/ViewDndContext.tsx";
import { useWebSocketStore, useMyPosition, useLowerPositionOnline, useAirport } from "@/store/store-hooks.ts";
import { useRef, useEffect, useMemo } from "react";
import { TWY_DEP_STRIP_WIDTH } from "@/components/strip/TwyDepStrip.tsx";

const scrollArea = "w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary";
const scrollAreaBottom = "w-full bg-[#555355] p-1 flex flex-col justify-end gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary";
const darkScrollArea = "w-full bg-[#212121] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary";
const darkScrollAreaBottom = "w-full bg-[#212121] p-1 flex flex-col justify-end gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary";
const btn = "bg-[#646464] text-white font-bold text-sm px-3 border-2 border-white active:bg-[#424242]";

export default function TWTE() {
  const myPosition = useMyPosition();
  const messages   = useActiveMessages();
  const lowerPositionOnline = useLowerPositionOnline();
  const airport = useAirport();

  const messagesEndRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  const finalAll = useFinalStrips();
  const rwyArrFlights = useMemo(
    () => finalAll.filter(isFlight).filter(s => s.destination === airport),
    [finalAll, airport]
  );
  const rwyArrSet = useMemo(
    () => new Set(rwyArrFlights.map(s => s.callsign)),
    [rwyArrFlights]
  );
  const finalBayStrips = useMemo(
    () => finalAll.filter(s => !isFlight(s) || !rwyArrSet.has(s.callsign)),
    [finalAll, rwyArrSet]
  );

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

  const updateOrder       = useWebSocketStore(state => state.updateOrder);
  const move              = useWebSocketStore(state => state.move);
  const moveTacticalStrip = useWebSocketStore(state => state.moveTacticalStrip);

  const ALL_ACTIVE = ["FINAL", "RWY-ARR", "TWY-ARR", "TWY-DEP", "RWY-DEP", "AIRBORNE", "STAND", "PUSHBACK", "DE-ICE"] as const;

  const bayStripMap = {
    "FINAL":    { strips: finalBayStrips, targetBay: Bay.Final },
    "RWY-ARR":  { strips: rwyArrFlights,  targetBay: Bay.Final },
    "TWY-ARR":  { strips: twyArrStrips,   targetBay: Bay.Taxi },
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
    "FINAL": "CLROK", "RWY-ARR": "CLROK",
    "TWY-ARR": "HALF", "TWY-DEP": "TWY-DEP",
    "RWY-DEP": "TWY-DEP", "AIRBORNE": "TWY-DEP",
    "STAND": "CLROK", "PUSHBACK": "PUSH", "DE-ICE": "PUSH",
  };
  const halfVariantForBay: Partial<Record<string, HalfStripVariant>> = {
    "TWY-ARR": "APN-ARR",
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
            halfStripVariant={halfVariantForBay[bayId]}
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
            <button className={btn}>NEW</button>
            <button className={btn}>MEM AID</button>
            <CrossingButton bay={Bay.Final} className={btn} />
          </span>
        </div>
        <SortableBay
          strips={finalBayStrips}
          bayId="FINAL"
          standalone={false}
          className={`h-[35%] ${scrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="CLROK" myPosition={myPosition} width={TWY_DEP_STRIP_WIDTH} />
          )}
        </SortableBay>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 border-t-4 border-[#A9A9A9]">
          <span className="text-white font-bold text-lg">RWY ARR</span>
        </div>
        <SortableBay
          strips={rwyArrFlights}
          bayId="RWY-ARR"
          standalone={false}
          className={`h-[20%] ${darkScrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="CLROK" myPosition={myPosition} />
          )}
        </SortableBay>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 border-t-4 border-[#A9A9A9] justify-between">
          <span className="text-white font-bold text-lg">TWY ARR</span>
          <span className="flex gap-1">
            <MemAidButton bay={Bay.Taxi} className={btn} />
            <CrossingButton bay={Bay.Taxi} className={btn} />
            <StartButton bay={Bay.Taxi} className={btn} />
            <LandButton bay={Bay.Taxi} className={btn} />
          </span>
        </div>
        <div className={`flex-1 ${scrollArea}`}>
          <SortableBay
            strips={twyArrStrips}
            bayId="TWY-ARR"
            standalone={false}
          >
            {(strip) => (
              <Strip strip={strip} status="HALF" halfStripVariant="APN-ARR" myPosition={myPosition} />
            )}
          </SortableBay>
        </div>
      </div>

      {/* Column 2 – TWY DEP + RWY DEP + AIRBORNE */}
      <div className="w-[28.5%] h-full bg-[#555355] flex flex-col">
        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 justify-between">
          <span className="text-white font-bold text-lg">TWY DEP</span>
          <span className="flex gap-1">
            <button className={btn}>STARTUP</button>
            <MemAidButton bay={Bay.Taxi} className={btn} />
            <CrossingButton bay={Bay.Taxi} className={btn} />
            <StartButton bay={Bay.Taxi} className={btn} />
            <LandButton bay={Bay.Taxi} className={btn} />
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
            <button className={btn}>NEW</button>
            <button className={btn}>MEM AID</button>
            <CrossingButton bay={Bay.Depart} className={btn} />
            <LandButton bay={Bay.Depart} className={btn} />
            <StartButton bay={Bay.Depart} className={btn} />
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
            <button className={btn}>INFO</button>
            <button className={btn}>MISC.</button>
            <button className={btn}>EQUIP</button>
          </span>
        </div>
        <div className={`flex-1 ${scrollArea}`}>
          {messages.map((msg, i) => (
            <Message key={i} from={msg.from}>{msg.message}</Message>
          ))}
          <div ref={messagesEndRef} />
        </div>
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
            <Strip key={s.callsign} strip={s} status={lowerPositionOnline ? "CLX-HALF" : "CLROK"} myPosition={myPosition} />
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
