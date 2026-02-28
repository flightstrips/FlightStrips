import { FlightStrip } from "@/components/strip/FlightStrip.tsx";
import { Message } from "@/components/Message.tsx";
import {
  useActiveMessages,
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
import { useRef, useEffect } from "react";


export default function GEGW() {
  const myPosition = useMyPosition();
  const messages     = useActiveMessages();

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
    />
  );
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  const finalStrips  = useFinalStrips();
  const rwyArrStrips = useRwyArrStrips();
  const twyArrStrips = useTaxiArrStrips();
  const pushStrips   = usePushbackStrips();
  const twyDepStrips = useTaxiDepStrips();
  const standStrips  = useStandStrips();
  const updateOrder  = useWebSocketStore(state => state.updateOrder);
  const move         = useWebSocketStore(state => state.move);

  // PUSHBACK, TWY-DEP-UPR, TWY-DEP-LWR, STAND are all draggable between each other.
  // TWY-DEP-UPR and TWY-DEP-LWR share Bay.Taxi (same backend bay, visual distinction only).
  const bayStripMap = {
    "PUSHBACK":    { strips: pushStrips,   targetBay: Bay.Push },
    "TWY-DEP-UPR": { strips: twyDepStrips, targetBay: Bay.Taxi },
    "TWY-DEP-LWR": { strips: twyDepStrips, targetBay: Bay.Taxi },
    "STAND":       { strips: standStrips,  targetBay: Bay.Stand },
  };

  const transferRules: Record<string, string[]> = {
    "PUSHBACK":    ["TWY-DEP-UPR", "TWY-DEP-LWR", "STAND"],
    "TWY-DEP-UPR": ["PUSHBACK",    "TWY-DEP-LWR", "STAND"],
    "TWY-DEP-LWR": ["PUSHBACK",    "TWY-DEP-UPR", "STAND"],
    "STAND":       ["PUSHBACK",    "TWY-DEP-UPR", "TWY-DEP-LWR"],
  };

  return (
    <ViewDndContext
      bayStripMap={bayStripMap}
      transferRules={transferRules}
      onReorder={updateOrder}
      onMove={move}
      renderDragOverlay={(callsign) => {
        const push = pushStrips.find(s => s.callsign === callsign);
        if (push) return mapToStrip(push, "HALF", "APN-PUSH");
        const twyDep = twyDepStrips.find(s => s.callsign === callsign);
        if (twyDep) return mapToStrip(twyDep, "CLROK");
        const stand = standStrips.find(s => s.callsign === callsign);
        if (stand) return mapToStrip(stand, "CLROK");
        return null;
      }}
    >
    <div className="bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center gap-2">

      {/* Column 1 (27%) – MESSAGES + FINAL + RWY ARR + TWY ARR */}
      <div className="w-[27%] h-full bg-[#555355] flex flex-col">
        <div className="bg-primary h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">MESSAGES</span>
        </div>
        <div className="h-[15%] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {messages.map((msg, i) => (
            <Message key={i} from={msg.from}>{msg.message}</Message>
          ))}
          <div ref={messagesEndRef} />
        </div>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">FINAL</span>
        </div>
        <DropIndicatorBay bayId="FINAL" className="h-[25%] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {finalStrips.map(x => mapToStrip(x, "HALF", "LOCKED-ARR", false))}
        </DropIndicatorBay>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">RWY ARR</span>
        </div>
        <DropIndicatorBay bayId="RWY-ARR" className="h-[20%] w-full bg-[#212121] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {rwyArrStrips.map(x => mapToStrip(x, "HALF", "LOCKED-ARR", false))}
        </DropIndicatorBay>

        {/* TWY ARR is SI-only; no manual drag */}
        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">TWY ARR</span>
        </div>
        <DropIndicatorBay bayId="TWY-ARR" className="flex-1 w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {twyArrStrips.map(x => mapToStrip(x, "HALF", "APN-ARR"))}
        </DropIndicatorBay>
      </div>

      {/* Column 2 (28%) – PUSHBACK + TWY DEP UPR + TWY DEP LWR (all draggable) */}
      <div className="w-[28%] h-full bg-[#555355] flex flex-col">
        <div className="bg-[#b3b3b3] h-10 flex items-center px-2 shrink-0">
          <span className="text-[#393939] font-bold text-lg">PUSHBACK</span>
        </div>
        <SortableBay
          strips={pushStrips}
          bayId="PUSHBACK"
          standalone={false}
          className="h-[20%] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary"
        >
          {(callsign) => {
            const strip = pushStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "HALF", "APN-PUSH");
          }}
        </SortableBay>

        <div className="bg-[#b3b3b3] h-10 flex items-center px-2 shrink-0">
          <span className="text-[#393939] font-bold text-lg">TWY DEP UPR</span>
        </div>
        <SortableBay
          strips={twyDepStrips}
          bayId="TWY-DEP-UPR"
          standalone={false}
          className="h-[35%] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary"
        >
          {(callsign) => {
            const strip = twyDepStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "CLROK");
          }}
        </SortableBay>

        <div className="bg-[#b3b3b3] h-10 flex items-center px-2 shrink-0">
          <span className="text-[#393939] font-bold text-lg">TWY DEP LWR</span>
        </div>
        <SortableBay
          strips={twyDepStrips}
          bayId="TWY-DEP-LWR"
          standalone={false}
          className="flex-1 w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary"
        >
          {(callsign) => {
            const strip = twyDepStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "CLROK");
          }}
        </SortableBay>
      </div>

      {/* Column 3 (25%) – CLRDEL (locked, no strips) */}
      <div className="w-1/4 h-full bg-[#555355] flex flex-col">
        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">CLRDEL</span>
        </div>
        <div className="flex-1 w-full bg-[#555355]" />
      </div>

      {/* Column 4 (20%) – STAND (draggable) */}
      <div className="w-1/5 h-full bg-[#555355] flex flex-col">
        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">STAND</span>
        </div>
        <SortableBay
          strips={standStrips}
          bayId="STAND"
          standalone={false}
          className="flex-1 w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary"
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
