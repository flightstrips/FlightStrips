import { Strip } from "@/components/strip/Strip.tsx";
import { MessageStrip } from "@/components/strip/MessageStrip.tsx";
import { MessageComposeDialog } from "@/components/MessageComposeDialog.tsx";
import {useClearedStrips, useNorwegianBayStrips, useOtherBayStrips, usePushbackStrips, useSasBayStrips, useTaxiDepStrips, isFlight} from "@/store/airports/ekch.ts";
import type {FrontendStrip} from "@/api/models.ts";
import { useMessages, useMyPosition } from "@/store/store-hooks.ts";
import { useState } from "react";
import { CLS_BTN, CLS_SCROLLBAR } from "@/components/strip/shared";
import { NewIfrDialog } from "@/components/strip/NewIfrDialog";
import { PlannedDialog } from "@/components/strip/PlannedDialog";

// Column widths — all four columns are equal
const W_COL = "w-1/4";
const col         = `${W_COL} h-full bg-[#555355]`; // column wrapper (no flex-col; each column manages its own layout)
const pageWrapper = "bg-[#A9A9A9] w-screen h-[calc(100vh-60px)] flex justify-center justify-items-center gap-2 aspect-video";

// Header class strings
const lockedHeader  = "bg-[#393939] h-10 flex items-center px-2 justify-between";
const lockedLabel   = "text-white font-bold text-lg";
const activeHeader  = "bg-[#b3b3b3] h-10 flex items-center px-2 justify-between";
const activeLabel   = "text-[#393939] font-bold text-lg";
const primaryHeader = "bg-primary h-10 flex items-center px-2 justify-between";
const primaryLabel  = "text-gray-100 font-bold text-lg";

// Scroll container classes
const scrollArea       = `w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto ${CLS_SCROLLBAR}`;
const scrollAreaBottom = `w-full bg-[#555355] p-1 flex flex-col justify-end gap-px overflow-y-auto ${CLS_SCROLLBAR}`;
const scrollAreaRaw = `w-full bg-[#555355] overflow-y-auto ${CLS_SCROLLBAR}`;

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
      <div className={pageWrapper}>
        <div className={col}>
          <div className={lockedHeader}>
            <span className={lockedLabel}>OTHERS</span>
            <span className="flex gap-2">
              <button className={CLS_BTN} onClick={() => setNewOpen(true)}>NEW</button>
              <button className={CLS_BTN} onClick={() => setPlannedOpen(true)}>PLANNED</button>
            </span>
          </div>
          <div className={`h-[calc(100%-2.5rem)] ${scrollArea}`}>
            {otherStrips.map(strip => mapToStrip(strip, "CLR"))}
          </div>
        </div>
        <div className={col}>
          <div className={lockedHeader}>
            <span className={lockedLabel}>SAS</span>
          </div>
          <div className={`h-[calc(67%-2.5rem)] ${scrollArea}`}>
            {sasStrips.map(strip => mapToStrip(strip, "CLR"))}
          </div>
          <div className={lockedHeader}>
            <span className={lockedLabel}>NORWEGIAN</span>
          </div>
          <div className={`h-[calc(33%-2.5rem)] ${scrollArea}`}>
            {norgewianStrips.map(strip => mapToStrip(strip, "CLR"))}
          </div>
        </div>
        <div className={col}>
          <div className={lockedHeader}>
            <span className={primaryLabel}>CLEARED</span>
          </div>
          <div className={`h-[calc(67%-2.5rem)] ${scrollArea}`}>
            {cleared.map(strip => mapToStrip(strip, "CLROK"))}
          </div>
          <div className={primaryHeader}>
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
          <div className={activeHeader}>
            <span className={activeLabel}>PUSHBACK</span>
          </div>
          <div className={`h-2/5 ${scrollAreaBottom}`}>
            {pushback.map(strip => mapToHalfStrip(strip))}
          </div>
          <div className={activeHeader}>
            <span className={activeLabel}>TWY DEP</span>
          </div>
          <div className={`h-[calc(60%-5rem)] ${scrollAreaBottom}`}>
            {taxidep.map(strip => mapToHalfStrip(strip))}
          </div>
        </div>
      </div>

      <NewIfrDialog open={newOpen} onOpenChange={setNewOpen} />
      <PlannedDialog open={plannedOpen} onOpenChange={setPlannedOpen} />
    </>
  );
}

