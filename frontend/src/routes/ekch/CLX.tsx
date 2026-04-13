import { Strip } from "@/components/strip/Strip.tsx";
import { MessageStrip } from "@/components/strip/MessageStrip.tsx";
import { MessageComposeDialog } from "@/components/MessageComposeDialog.tsx";
import {useClearedStrips, useNorwegianBayStrips, useOtherBayStrips, usePushbackStrips, useSasBayStrips, useTaxiDepStrips, isFlight} from "@/store/airports/ekch.ts";
import type {FrontendStrip} from "@/api/models.ts";
import { useMessages, useMyPosition } from "@/store/store-hooks.ts";
import { useState } from "react";
import { CLS_BTN } from "@/components/strip/shared";
import { NewIfrDialog } from "@/components/strip/NewIfrDialog";
import { PlannedDialog } from "@/components/strip/PlannedDialog";

const col         = "w-1/4 bay-col";
const lockedLabel  = "text-white font-bold text-lg";
const activeLabel  = "text-bay-header font-bold text-lg";
const primaryLabel = "text-gray-100 font-bold text-lg";
const scrollAreaRaw = "w-full min-h-0 bg-bay-panel flex flex-col overflow-y-auto overscroll-y-contain [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary";

export default function DEL() {
  const myPosition = useMyPosition();
  const sasStrips = useSasBayStrips().sort((a, b) => a.sequence - b.sequence);
  const norgewianStrips = useNorwegianBayStrips().sort((a, b) => a.sequence - b.sequence);
  const otherStrips = useOtherBayStrips().sort((a, b) => a.sequence - b.sequence);
  const cleared = useClearedStrips().sort((a, b) => a.sequence - b.sequence);
  const pushback = usePushbackStrips().filter(isFlight).sort((a, b) => b.sequence - a.sequence);
  const taxidep = useTaxiDepStrips().filter(isFlight).sort((a, b) => b.sequence - a.sequence);
  const messages = useMessages();
  const [composeOpen, setComposeOpen] = useState(false);
  const [newOpen, setNewOpen] = useState(false);
  const [plannedOpen, setPlannedOpen] = useState(false);

  const mapToStrip = (strip: FrontendStrip, status: string) => (
    <Strip
      key={strip.callsign}
      strip={strip}
      status={status as "CLR" | "CLROK" | "HALF"}
      myPosition={myPosition}
      selectable={false}
    />
  );

  const mapToHalfStrip = (strip: FrontendStrip) => (
    <Strip key={strip.callsign} strip={strip} status="CLX-HALF" />
  );

  return (
    <>
      <div className="bay-page-wrapper aspect-video">
        <div className={col}>
          <div className="bay-col-header justify-between">
            <span className={lockedLabel}>OTHERS</span>
            <span className="flex gap-2">
              <button className={CLS_BTN} onClick={() => setNewOpen(true)}>NEW</button>
              <button className={CLS_BTN} onClick={() => setPlannedOpen(true)}>PLANNED</button>
            </span>
          </div>
          <div className="h-[calc(100%-2.5rem)] bay-scroll-area">
            {otherStrips.map(strip => mapToStrip(strip, "CLR"))}
          </div>
        </div>
        <div className={col}>
          <div className="bay-col-header justify-between">
            <span className={lockedLabel}>SAS</span>
          </div>
          <div className="h-[calc(67%-2.5rem)] bay-scroll-area">
            {sasStrips.map(strip => mapToStrip(strip, "CLR"))}
          </div>
          <div className="bay-col-header bay-col-sep justify-between">
            <span className={lockedLabel}>NORWEGIAN</span>
          </div>
          <div className="h-[calc(33%-2.5rem)] bay-scroll-area">
            {norgewianStrips.map(strip => mapToStrip(strip, "CLR"))}
          </div>
        </div>
        <div className={col}>
          <div className="bay-col-header justify-between">
            <span className={primaryLabel}>CLEARED</span>
          </div>
          <div className="h-[calc(67%-2.5rem)] bay-scroll-area">
            {cleared.map(strip => mapToStrip(strip, "CLROK"))}
          </div>
          <div className="bay-col-header-primary bay-col-sep justify-between">
            <span className={primaryLabel}>MESSAGES</span>
            <button className={CLS_BTN} onClick={() => setComposeOpen(true)}>FREE TEXT</button>
          </div>
          <div className={`h-[calc(33%-6rem)] ${scrollAreaRaw}`}>
            {messages.map(msg => (
              <MessageStrip key={msg.id} msg={msg} />
            ))}
          </div>
          <MessageComposeDialog open={composeOpen} onClose={() => setComposeOpen(false)} />
        </div>
        <div className={col}>
          <div className="bay-col-header-light justify-between">
            <span className={activeLabel}>PUSHBACK</span>
          </div>
          <div className="h-2/5 bay-scroll-area-bottom">
            {pushback.map(strip => mapToHalfStrip(strip))}
          </div>
          <div className="bay-col-header-light bay-col-sep justify-between">
            <span className={activeLabel}>TWY DEP</span>
          </div>
          <div className="h-[calc(60%-5rem)] bay-scroll-area-bottom">
            {taxidep.map(strip => mapToHalfStrip(strip))}
          </div>
        </div>
      </div>

      <NewIfrDialog open={newOpen} onOpenChange={setNewOpen} />
      <PlannedDialog open={plannedOpen} onOpenChange={setPlannedOpen} />
    </>
  );
}
