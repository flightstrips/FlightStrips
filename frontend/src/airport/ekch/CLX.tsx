
import { FlightStrip } from "@/components/strip/FlightStrip.tsx";
import { Message } from "@/components/Message.tsx";
import {useClearedStrips, useNorwegianBayStrips, useOtherBayStrips, useSasBayStrips} from "@/store/ekch.ts";
import type {FrontendStrip} from "@/api/models.ts";

export default function DEL() {
  const sasStrips = useSasBayStrips().sort((a, b) => a.sequence - b.sequence);
  const norgewianStrips = useNorwegianBayStrips().sort((a, b) => a.sequence - b.sequence);
  const otherStrips = useOtherBayStrips().sort((a, b) => a.sequence - b.sequence);
  const cleared = useClearedStrips().sort((a, b) => a.sequence - b.sequence);

  const mapToStrip = (strip: FrontendStrip, status: string) => <FlightStrip callsing={strip.callsign} destination={strip.destination} stand={strip.stand} eobt={strip.eobt} tsat={strip.tsat} ctot={strip.ctot} status={status} key={strip.callsign} pdcStatus={strip.pdc_state}/>

  return (
    <>
      <div className="bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center gap-2 aspect-video">
        <div className="w-1/4 h-full bg-[#555355]">
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">
              OTHERS
            </span>
            <span className="flex gap-2">
              <button className="bg-[#646464] text-white font-bold text-lg px-4 border-2 border-white active:bg-[#424242]">
                NEW
              </button>
              <button className="bg-[#646464] text-white font-bold text-lg px-4 border-2 border-white active:bg-[#424242]">
                PLANNED
              </button>
            </span>
          </div>
          <div className="h-[calc(100%-2.5rem)] w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
            {otherStrips.map(x => mapToStrip(x, "CLR"))}
          </div>
        </div>
        <div className="w-1/4 h-full bg-[#555355]">
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">
              SAS
            </span>
          </div>
          <div className="h-[calc(50%-2.5rem)] w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
            {sasStrips.map(x => mapToStrip(x, "CLR"))}
          </div>
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">
              NORWEGIAN
            </span>
          </div>
          <div className="h-[calc(50%-2.5rem)] w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
            {norgewianStrips.map(x => mapToStrip(x, "CLR"))}
          </div>
        </div>
        <div className="w-1/4 h-full bg-[#555355]">
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-gray-100 font-bold text-lg">
              CLEARED
            </span>
          </div>
          <div className="h-1/2 w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
            {cleared.map(x => mapToStrip(x, "CLROK"))}
          </div>
          <div className="bg-primary h-10 flex items-center px-2 justify-between">
            <span className="text-gray-100 font-bold text-lg">
              MESSAGES
            </span>
          </div>
          <div className="h-[calc(50%-6rem)] w-full bg-[#555355]">
            <Message>Lorem ipsum dolor sit amet, consectetur adipiscing elit. Nam tincidunt vitae enim eget porttitor. Suspendisse ultrices ullamcorper tortor, vitae condimentum lacus convallis at. </Message>
            <Message><b>FLIGHTSTRIPS</b> has deteced that EKCH_DEL has logged off. You are in change of Delivery!</Message>
            <Message>VFR Request LOW PASS rwy 12</Message>
          </div>
        </div>
        <div className="w-1/4 h-full bg-[#555355]">
          <div className="bg-[#b3b3b3] h-10 flex items-center px-2 justify-between">
            <span className="text-[#393939] font-bold text-lg">
              PUSHBACK
            </span>
          </div>
          <div className="h-2/5 w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
            <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF" />
            <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF" />
            <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF" />
            <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF" />
            <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF" />
          </div>
          <div className="bg-[#b3b3b3] h-10 flex items-center px-2 justify-between">
            <span className="text-[#393939] font-bold text-lg">
              TWY DEP
            </span>
          </div>
          <div className="h-[calc(60%-5rem)] w-full bg-[#555355] p-1 flex flex-col gap-[2px] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
            <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF" />
            <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF" />
            <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF" />
            <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF" />
            <FlightStrip callsing="NSZ1234" clearances standchanged taxiway="A" holdingpoint="B1" destination={'ESSA'} stand={'A6'} tsat={'1400'} status="HALF" />
          </div>
        </div>
      </div>
      
    </>
  );
}
