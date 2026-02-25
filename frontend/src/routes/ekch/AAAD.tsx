import { FlightStrip } from "@/components/strip/FlightStrip.tsx";
import {
  useActiveMessages,
  useClearedStrips,
  useFinalStrips,
  useNorwegianBayStrips,
  useOtherBayStrips,
  usePushbackStrips,
  useRwyArrStrips,
  useSasBayStrips,
  useTaxiArrStrips,
  useTaxiDepStrips,
} from "@/store/airports/ekch.ts";
import type { FrontendStrip } from "@/api/models.ts";
import type { HalfStripVariant, StripStatus } from "@/components/strip/types.ts";
import { SortableBay } from "@/components/bays/SortableBay.tsx";
import { useWebSocketStore } from "@/store/store-hooks.ts";

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

export default function AAAD() {
  const messages      = useActiveMessages();
  const finalStrips   = useFinalStrips();
  const rwyArrStrips  = useRwyArrStrips();
  const twyArrStrips  = useTaxiArrStrips();
  const startupStrips = useClearedStrips().sort((a, b) => a.sequence - b.sequence);
  const pushStrips    = usePushbackStrips();
  const twyDepStrips  = useTaxiDepStrips();
  const otherStrips   = useOtherBayStrips().sort((a, b) => a.sequence - b.sequence);
  const sasStrips     = useSasBayStrips().sort((a, b) => a.sequence - b.sequence);
  const norStrips     = useNorwegianBayStrips().sort((a, b) => a.sequence - b.sequence);
  const updateOrder   = useWebSocketStore(state => state.updateOrder);

  return (
    <div className="bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center gap-2">

      {/* Column 1 – Arrivals + Messages */}
      <div className="w-1/4 h-full bg-[#555355] flex flex-col">
        <div className="bg-primary h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">MESSAGES</span>
        </div>
        <div className="h-[20%] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {messages.length === 0 && null}
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
        <div className="flex-1 w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {rwyArrStrips.map(x => mapToStrip(x, "HALF", "LOCKED-ARR", false))}
        </div>
      </div>

      {/* Column 2 – TWY ARR + TWY DEP UPR */}
      <div className="w-1/4 h-full bg-[#555355] flex flex-col">
        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">TWY ARR</span>
        </div>
        <SortableBay
          strips={twyArrStrips}
          onReorder={updateOrder}
          className="h-[40%] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary"
        >
          {(callsign) => {
            const strip = twyArrStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "HALF", "APN-ARR");
          }}
        </SortableBay>

        <div className="bg-[#b3b3b3] h-10 flex items-center px-2 shrink-0">
          <span className="text-[#393939] font-bold text-lg">TWY DEP UPR</span>
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

      {/* Column 3 – STARTUP + PUSHBACK */}
      <div className="w-1/4 h-full bg-[#555355] flex flex-col">
        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">STARTUP</span>
        </div>
        <SortableBay
          strips={startupStrips}
          onReorder={updateOrder}
          className="h-[55%] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary"
        >
          {(callsign) => {
            const strip = startupStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "CLROK");
          }}
        </SortableBay>

        <div className="bg-[#b3b3b3] h-10 flex items-center px-2 shrink-0">
          <span className="text-[#393939] font-bold text-lg">PUSHBACK</span>
        </div>
        <SortableBay
          strips={pushStrips}
          onReorder={updateOrder}
          className="flex-1 w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary"
        >
          {(callsign) => {
            const strip = pushStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "HALF", "APN-PUSH");
          }}
        </SortableBay>
      </div>

      {/* Column 4 – OTHERS / SAS / NORWEGIAN (locked, uncleared) */}
      <div className="w-1/4 h-full bg-[#555355] flex flex-col">
        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">OTHERS</span>
        </div>
        <SortableBay
          strips={otherStrips}
          onReorder={updateOrder}
          className="h-[40%] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary"
        >
          {(callsign) => {
            const strip = otherStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "CLR", undefined, false);
          }}
        </SortableBay>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">SAS</span>
        </div>
        <SortableBay
          strips={sasStrips}
          onReorder={updateOrder}
          className="h-[25%] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary"
        >
          {(callsign) => {
            const strip = sasStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "CLR", undefined, false);
          }}
        </SortableBay>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">NORWEGIAN</span>
        </div>
        <SortableBay
          strips={norStrips}
          onReorder={updateOrder}
          className="flex-1 w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary"
        >
          {(callsign) => {
            const strip = norStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "CLR", undefined, false);
          }}
        </SortableBay>
      </div>

    </div>
  );
}
