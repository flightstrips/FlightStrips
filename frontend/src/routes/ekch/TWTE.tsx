import { FlightStrip } from "@/components/strip/FlightStrip.tsx";
import { Message } from "@/components/Message.tsx";
import {
  useActiveMessages,
  useAirborneStrips,
  useClearedStrips,
  useDepartStrips,
  useDeIceStrips,
  useFinalStrips,
  usePushbackStrips,
  useRwyArrStrips,
  useStandStrips,
  useTaxiArrStrips,
  useTaxiDepStrips,
} from "@/store/airports/ekch.ts";
import type { FrontendStrip } from "@/api/models.ts";
import { Bay } from "@/api/models.ts";
import type { HalfStripVariant, StripStatus } from "@/components/strip/types.ts";
import { SortableBay, DropIndicatorBay } from "@/components/bays/SortableBay.tsx";
import { ViewDndContext } from "@/components/bays/ViewDndContext.tsx";
import { useWebSocketStore, useMyPosition } from "@/store/store-hooks.ts";
import { useRef, useEffect, useMemo } from "react";

const scrollArea = "w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary";
const darkScrollArea = "w-full bg-[#212121] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary";
const btn = "bg-[#646464] text-white font-bold text-sm px-3 border-2 border-white active:bg-[#424242]";

export default function TWTE() {
  const myPosition = useMyPosition();
  const messages   = useActiveMessages();

  const mapToStrip = (strip: FrontendStrip, status: StripStatus, halfStripVariant?: HalfStripVariant, selectable = true) => (
    <FlightStrip
      key={strip.callsign}
      callsign={strip.callsign}
      status={status}
      halfStripVariant={halfStripVariant}
      pdcStatus={strip.pdc_state}
      destination={strip.destination}
      origin={strip.origin}
      stand={strip.stand}
      eobt={strip.eobt}
      tobt={strip.tobt}
      tsat={strip.tsat}
      ctot={strip.ctot}
      aircraftType={strip.aircraft_type}
      squawk={strip.squawk}
      sid={strip.sid}
      runway={strip.runway}
      clearedAltitude={strip.cleared_altitude}
      requestedAltitude={strip.requested_altitude}
      holdingPoint={strip.release_point}
      owner={strip.owner}
      nextControllers={strip.next_controllers}
      previousControllers={strip.previous_controllers}
      myPosition={myPosition}
      selectable={selectable}
      marked={strip.marked}
    />
  );
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  const finalStrips   = useFinalStrips();
  const rwyArrStrips  = useRwyArrStrips();
  const twyArrStrips  = useTaxiArrStrips();
  const twyDepStrips  = useTaxiDepStrips();
  const rwyDepStrips  = useDepartStrips();
  const airborne      = useAirborneStrips();
  const standStrips   = useStandStrips();
  const pushStrips    = usePushbackStrips();
  const clearedStrips = useClearedStrips();
  const deIceStrips   = useDeIceStrips();
  const updateOrder   = useWebSocketStore(state => state.updateOrder);
  const move          = useWebSocketStore(state => state.move);

  // RWY-ARR is a subset of FINAL (Bay.Final filtered by destination=airport).
  // Give RWY-ARR strips exclusive ownership so they are correctly identified
  // as "from RWY-ARR" rather than "from FINAL" during drag.
  const rwyArrCallsigns = useMemo(
    () => new Set(rwyArrStrips.map(s => s.callsign)),
    [rwyArrStrips]
  );
  const finalOnlyStrips = useMemo(
    () => finalStrips.filter(s => !rwyArrCallsigns.has(s.callsign)),
    [finalStrips, rwyArrCallsigns]
  );

  const ALL_ACTIVE = ["FINAL", "RWY-ARR", "TWY-ARR", "TWY-DEP", "RWY-DEP", "AIRBORNE", "STAND", "PUSHBACK", "DE-ICE"] as const;

  const bayStripMap = {
    "FINAL":    { strips: finalOnlyStrips, targetBay: Bay.Final },
    "RWY-ARR":  { strips: rwyArrStrips,   targetBay: Bay.Final },
    "TWY-ARR":  { strips: twyArrStrips,   targetBay: Bay.Taxi },
    "TWY-DEP":  { strips: twyDepStrips,   targetBay: Bay.Taxi },
    "RWY-DEP":  { strips: rwyDepStrips,   targetBay: Bay.Depart },
    "AIRBORNE": { strips: airborne,       targetBay: Bay.Airborne },
    "STAND":    { strips: standStrips,    targetBay: Bay.Stand },
    "PUSHBACK": { strips: pushStrips,     targetBay: Bay.Push },
    "DE-ICE":   { strips: deIceStrips,   targetBay: Bay.DeIce },
  };

  const transferRules: Record<string, string[]> = {
    // FINAL may only go to RWY-ARR or TWY-ARR
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

  return (
    <ViewDndContext
      bayStripMap={bayStripMap}
      transferRules={transferRules}
      onReorder={updateOrder}
      onMove={move}
      renderDragOverlay={(callsign) => {
        const final = finalStrips.find(s => s.callsign === callsign);
        if (final) return mapToStrip(final, "CLROK");
        const rwyArr = rwyArrStrips.find(s => s.callsign === callsign);
        if (rwyArr) return mapToStrip(rwyArr, "CLROK");
        const twyArr = twyArrStrips.find(s => s.callsign === callsign);
        if (twyArr) return mapToStrip(twyArr, "HALF", "APN-ARR");
        const twyDep = twyDepStrips.find(s => s.callsign === callsign);
        if (twyDep) return mapToStrip(twyDep, "CLROK");
        const rwyDep = rwyDepStrips.find(s => s.callsign === callsign);
        if (rwyDep) return mapToStrip(rwyDep, "CLROK");
        const air = airborne.find(s => s.callsign === callsign);
        if (air) return mapToStrip(air, "CLROK");
        const stand = standStrips.find(s => s.callsign === callsign);
        if (stand) return mapToStrip(stand, "CLROK");
        const push = pushStrips.find(s => s.callsign === callsign);
        if (push) return mapToStrip(push, "PUSH");
        const deIce = deIceStrips.find(s => s.callsign === callsign);
        if (deIce) return mapToStrip(deIce, "PUSH");
        return null;
      }}
    >
    <div className="bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center gap-2">

      {/* Column 1 (27%) – FINAL + RWY ARR + TWY ARR */}
      <div className="w-[27%] h-full bg-[#555355] flex flex-col">
        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 justify-between">
          <span className="text-white font-bold text-lg">FINAL</span>
          <span className="flex gap-1">
            <button className={btn}>NEW</button>
            <button className={btn}>MEM AID</button>
          </span>
        </div>
        <SortableBay
          strips={finalStrips}
          bayId="FINAL"
          standalone={false}
          className={`h-[35%] ${scrollArea}`}
        >
          {(callsign) => {
            const strip = finalStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "CLROK", undefined, true);
          }}
        </SortableBay>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 border-t-4 border-[#A9A9A9]">
          <span className="text-white font-bold text-lg">RWY ARR</span>
        </div>
        <SortableBay
          strips={rwyArrStrips}
          bayId="RWY-ARR"
          standalone={false}
          className={`h-[20%] ${darkScrollArea}`}
        >
          {(callsign) => {
            const strip = rwyArrStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "CLROK");
          }}
        </SortableBay>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 border-t-4 border-[#A9A9A9]">
          <span className="text-white font-bold text-lg">TWY ARR</span>
        </div>
        <SortableBay
          strips={twyArrStrips}
          bayId="TWY-ARR"
          standalone={false}
          className={`flex-1 ${scrollArea}`}
        >
          {(callsign) => {
            const strip = twyArrStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "HALF", "APN-ARR");
          }}
        </SortableBay>
      </div>

      {/* Column 2 (28%) – TWY DEP + RWY DEP + AIRBORNE */}
      <div className="w-[28%] h-full bg-[#555355] flex flex-col">
        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 justify-between">
          <span className="text-white font-bold text-lg">TWY DEP</span>
          <span className="flex gap-1">
            <button className={btn}>STARTUP</button>
          </span>
        </div>
        <SortableBay
          strips={twyDepStrips}
          bayId="TWY-DEP"
          standalone={false}
          className={`h-[35%] ${scrollArea}`}
        >
          {(callsign) => {
            const strip = twyDepStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "CLROK");
          }}
        </SortableBay>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 justify-between border-t-4 border-[#A9A9A9]">
          <span className="text-white font-bold text-lg">RWY DEP</span>
          <span className="flex gap-1">
            <button className={btn}>NEW</button>
            <button className={btn}>MEM AID</button>
            <button className={btn}>LAND</button>
            <button className={btn}>START</button>
          </span>
        </div>
        <SortableBay
          strips={rwyDepStrips}
          bayId="RWY-DEP"
          standalone={false}
          className={`h-[20%] ${darkScrollArea}`}
        >
          {(callsign) => {
            const strip = rwyDepStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "CLROK");
          }}
        </SortableBay>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 border-t-4 border-[#A9A9A9]">
          <span className="text-white font-bold text-lg">AIRBORNE</span>
        </div>
        <SortableBay
          strips={airborne}
          bayId="AIRBORNE"
          standalone={false}
          className={`flex-1 ${scrollArea}`}
        >
          {(callsign) => {
            const strip = airborne.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "CLROK");
          }}
        </SortableBay>
      </div>

      {/* Column 3 (25%) – CONTROLZONE + PUSHBACK + MESSAGES */}
      <div className="w-1/4 h-full bg-[#555355] flex flex-col">
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
          {(callsign) => {
            const strip = pushStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "PUSH");
          }}
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

      {/* Column 4 (20%) – CLRDEL + DE-ICE A + STAND */}
      <div className="w-1/5 h-full bg-[#555355] flex flex-col">
        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 justify-between">
          <span className="text-white font-bold text-lg">CLRDEL</span>
          <span className="flex gap-1">
            <button className={btn}>NEW</button>
            <button className={btn}>PLANNED</button>
          </span>
        </div>
        <DropIndicatorBay bayId="CLRDEL" className={`h-[45%] ${scrollArea}`}>
          {clearedStrips.map(x => mapToStrip(x, "CLROK", undefined, false))}
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
          {(callsign) => {
            const strip = deIceStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "PUSH");
          }}
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
          {(callsign) => {
            const strip = standStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "CLROK");
          }}
        </SortableBay>
      </div>

    </div>
    </ViewDndContext>
  );
}
