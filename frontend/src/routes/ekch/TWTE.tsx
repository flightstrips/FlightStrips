import { FlightStrip } from "@/components/strip/FlightStrip.tsx";
import {
  useActiveMessages,
  useAirborneStrips,
  useDepartStrips,
  useFinalStrips,
  useRwyArrStrips,
  useStandStrips,
  useTaxiArrStrips,
  useTaxiDepStrips,
} from "@/store/airports/ekch.ts";
import type { FrontendStrip } from "@/api/models.ts";
import type { HalfStripVariant, StripStatus } from "@/components/strip/types.ts";

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

export default function TWTE() {
  const messages     = useActiveMessages();
  const finalStrips  = useFinalStrips();
  const rwyArrStrips = useRwyArrStrips();
  const twyArrStrips = useTaxiArrStrips();
  const twyDepStrips = useTaxiDepStrips();
  const rwyDepStrips = useDepartStrips();
  const airborne     = useAirborneStrips();
  const standStrips  = useStandStrips();

  return (
    <div className="bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center gap-2">

      {/* Column 1 (27%) – FINAL + RWY ARR + TWY ARR (wider arrival column) */}
      <div className="w-[27%] h-full bg-[#555355] flex flex-col">
        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">FINAL</span>
        </div>
        <div className="h-[35%] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {finalStrips.map(x => mapToStrip(x, "CLROK", undefined, true))}
        </div>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">RWY ARR</span>
        </div>
        <div className="h-[20%] w-full bg-[#212121] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {rwyArrStrips.map(x => mapToStrip(x, "CLROK"))}
        </div>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">TWY ARR</span>
        </div>
        <div className="flex-1 w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {twyArrStrips.map(x => mapToStrip(x, "HALF", "APN-ARR"))}
        </div>
      </div>

      {/* Column 2 (28%) – TWY DEP + RWY DEP + AIRBORNE */}
      <div className="w-[28%] h-full bg-[#555355] flex flex-col">
        <div className="bg-[#b3b3b3] h-10 flex items-center px-2 shrink-0">
          <span className="text-[#393939] font-bold text-lg">TWY DEP</span>
        </div>
        <div className="h-[55%] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {twyDepStrips.map(x => mapToStrip(x, "CLROK"))}
        </div>

        <div className="bg-[#b3b3b3] h-10 flex items-center px-2 shrink-0">
          <span className="text-[#393939] font-bold text-lg">RWY DEP</span>
        </div>
        <div className="h-[20%] w-full bg-[#212121] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {rwyDepStrips.map(x => mapToStrip(x, "CLROK"))}
        </div>

        <div className="bg-[#b3b3b3] h-10 flex items-center px-2 shrink-0">
          <span className="text-[#393939] font-bold text-lg">AIRBORNE</span>
        </div>
        <div className="flex-1 w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {airborne.map(x => mapToStrip(x, "CLROK"))}
        </div>
      </div>

      {/* Column 3 (25%) – CONTROLZONE + DE-ICE + MESSAGES */}
      <div className="w-1/4 h-full bg-[#555355] flex flex-col">
        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">CONTROLZONE</span>
        </div>
        {/* VFR strips – bay TBD with backend */}
        <div className="h-[35%] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary" />

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">DE-ICE</span>
        </div>
        {/* De-ice strips – bay TBD with backend */}
        <div className="h-[35%] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary" />

        <div className="bg-primary h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">MESSAGES</span>
        </div>
        <div className="flex-1 w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {messages.length === 0 && null}
        </div>
      </div>

      {/* Column 4 (20%) – CLRDEL (locked reference) + STAND */}
      <div className="w-1/5 h-full bg-[#555355] flex flex-col">
        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">CLRDEL</span>
        </div>
        {/* Locked reference column – no strips rendered */}
        <div className="h-[80%] w-full bg-[#555355]" />

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">STAND</span>
        </div>
        <div className="flex-1 w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {standStrips.map(x => mapToStrip(x, "CLROK"))}
        </div>
      </div>

    </div>
  );
}
