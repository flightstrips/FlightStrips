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
import type { HalfStripVariant, StripStatus } from "@/components/strip/types.ts";
import { SortableBay } from "@/components/bays/SortableBay.tsx";
import { useWebSocketStore } from "@/store/store-hooks.ts";
import { useRef, useEffect } from "react";

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
    owner={strip.owner}
    selectable={selectable}
  />
);

export default function GEGW() {
  const messages     = useActiveMessages();
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

  return (
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
        <div className="h-[25%] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {finalStrips.map(x => mapToStrip(x, "HALF", "LOCKED-ARR", false))}
        </div>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">RWY ARR</span>
        </div>
        <div className="h-[20%] w-full bg-[#212121] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {rwyArrStrips.map(x => mapToStrip(x, "HALF", "LOCKED-ARR", false))}
        </div>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">TWY ARR</span>
        </div>
        <SortableBay
          strips={twyArrStrips}
          onReorder={updateOrder}
          className="flex-1 w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary"
        >
          {(callsign) => {
            const strip = twyArrStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "HALF", "APN-ARR");
          }}
        </SortableBay>
      </div>

      {/* Column 2 (28%) – PUSHBACK + TWY DEP UPR + TWY DEP LWR */}
      <div className="w-[28%] h-full bg-[#555355] flex flex-col">
        <div className="bg-[#b3b3b3] h-10 flex items-center px-2 shrink-0">
          <span className="text-[#393939] font-bold text-lg">PUSHBACK</span>
        </div>
        <SortableBay
          strips={pushStrips}
          onReorder={updateOrder}
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
          onReorder={updateOrder}
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
          onReorder={updateOrder}
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

      {/* Column 4 (20%) – STAND */}
      <div className="w-1/5 h-full bg-[#555355] flex flex-col">
        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">STAND</span>
        </div>
        <SortableBay
          strips={standStrips}
          onReorder={updateOrder}
          className="flex-1 w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary"
        >
          {(callsign) => {
            const strip = standStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "CLROK");
          }}
        </SortableBay>
      </div>

    </div>
  );
}
