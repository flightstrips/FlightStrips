import { Strip } from "@/components/strip/Strip.tsx";
import { MessageStrip } from "@/components/strip/MessageStrip.tsx";
import { MessageComposeDialog } from "@/components/MessageComposeDialog.tsx";
import {useClearedStrips, useNorwegianBayStrips, useOtherBayStrips, usePushbackStrips, useSasBayStrips, useTaxiDepStrips, isFlight} from "@/store/airports/ekch.ts";
import type {FrontendStrip} from "@/api/models.ts";
import { useMessages, useMyPosition } from "@/store/store-hooks.ts";
import { useState } from "react";

export default function DEL() {
  const myPosition = useMyPosition();
  const sasStrips = useSasBayStrips().sort((a, b) => a.sequence - b.sequence);
  const norgewianStrips = useNorwegianBayStrips().sort((a, b) => a.sequence - b.sequence);
  const otherStrips = useOtherBayStrips().sort((a, b) => a.sequence - b.sequence);
  const cleared = useClearedStrips().sort((a, b) => a.sequence - b.sequence);
  const pushback = usePushbackStrips().filter(isFlight).sort((a, b) => a.sequence - b.sequence);
  const taxidep = useTaxiDepStrips().filter(isFlight).sort((a, b) => a.sequence - b.sequence);
  const messages = useMessages();
  const [composeOpen, setComposeOpen] = useState(false);

  const mapToStrip = (strip: FrontendStrip, status: string) => (
    <Strip
      key={strip.callsign}
      strip={strip}
      status={status as "CLR" | "CLROK" | "HALF"}
      myPosition={myPosition}
    />
  );

  const mapToHalfStrip = (strip: FrontendStrip) => (
    <Strip key={strip.callsign} strip={strip} status="CLX-HALF" />
  );

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
          <div className="h-[calc(100%-2.5rem)] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
            {otherStrips.map(strip => mapToStrip(strip, "CLR"))}
          </div>
        </div>
        <div className="w-1/4 h-full bg-[#555355]">
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">
              SAS
            </span>
          </div>
          <div className="h-[calc(67%-2.5rem)] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
            {sasStrips.map(strip => mapToStrip(strip, "CLR"))}
          </div>
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-white font-bold text-lg">
              NORWEGIAN
            </span>
          </div>
          <div className="h-[calc(33%-2.5rem)] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
            {norgewianStrips.map(strip => mapToStrip(strip, "CLR"))}
          </div>
        </div>
        <div className="w-1/4 h-full bg-[#555355]">
          <div className="bg-[#393939] h-10 flex items-center px-2 justify-between">
            <span className="text-gray-100 font-bold text-lg">
              CLEARED
            </span>
          </div>
          <div className="h-[calc(67%-2.5rem)] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
            {cleared.map(strip => mapToStrip(strip, "CLROK"))}
          </div>
          <div className="bg-primary h-10 flex items-center px-2 justify-between">
            <span className="text-gray-100 font-bold text-lg">MESSAGES</span>
            <button className="bg-[#646464] text-white font-bold text-sm px-3 border-2 border-white active:bg-[#424242]" onClick={() => setComposeOpen(true)}>FREE TEXT</button>
          </div>
          <div className="h-[calc(33%-6rem)] w-full bg-[#555355] overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
            {messages.map(msg => (
              <MessageStrip key={msg.id} msg={msg} />
            ))}
          </div>
          <MessageComposeDialog open={composeOpen} onClose={() => setComposeOpen(false)} />
        </div>
        <div className="w-1/4 h-full bg-[#555355]">
          <div className="bg-[#b3b3b3] h-10 flex items-center px-2 justify-between">
            <span className="text-[#393939] font-bold text-lg">
              PUSHBACK
            </span>
          </div>
          <div className="h-2/5 w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
            {pushback.map(strip => mapToHalfStrip(strip))}

          </div>
          <div className="bg-[#b3b3b3] h-10 flex items-center px-2 justify-between">
            <span className="text-[#393939] font-bold text-lg">
              TWY DEP
            </span>
          </div>
          <div className="h-[calc(60%-5rem)] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
            {taxidep.map(strip => mapToHalfStrip(strip))}
          </div>
        </div>
      </div>
      
    </>
  );
}

