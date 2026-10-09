import { Strip } from "@/components/strip/Strip.tsx";
import { MessageStrip } from "@/components/strip/MessageStrip.tsx";
import { MessageComposeDialog } from "@/components/MessageComposeDialog.tsx";
import {useClearedStrips, useNorwegianBayStrips, useOtherBayStrips, usePushbackStrips, useSasBayStrips, useTaxiDepStrips, isFlight} from "@/store/airports/ekch.ts";
import {stripDndId, type AnyStrip, type FrontendStrip} from "@/api/models.ts";
import { useMessages, useMyPosition } from "@/store/store-hooks.ts";
import { useState } from "react";
import { CLS_BTN_MISSED, CLS_BTN_NEW, CLS_BTN_PLANNED } from "@/components/strip/shared";
import { NewIfrDialog } from "@/components/strip/NewIfrDialog";
import { PlannedDialog } from "@/components/strip/PlannedDialog";
import { useBayResize } from "@/components/bays/useBayResize";
import { BayResizeHandle } from "@/components/bays/BayResizeHandle";
import { AutoAlignedBay } from "@/components/bays/SortableBay";

const col         = "w-1/4 bay-col";
const lockedLabel  = "text-[#CECECE] font-bay tracking-[0.06em] [-webkit-text-stroke:0.5px_currentColor] text-[1.11375rem]";
const activeLabel  = "text-bay-header font-bold text-lg";
const primaryLabel = "text-[#CECECE] font-bay tracking-[0.06em] [-webkit-text-stroke:0.5px_currentColor] text-[1.11375rem]";
const scrollAreaRaw = "w-full min-h-0 bg-bay-panel flex flex-col overflow-y-auto overscroll-y-contain [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary";

// Default bay heights (% of column); the last bay of each column fills the rest.
const SAS_DEFAULTS = { sas: 62 };
const CLEARED_DEFAULTS = { cleared: 62 };
const PUSHBACK_DEFAULTS = { pushback: 40 };

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
  const sasResize = useBayResize("clx-bay-heights-sas", SAS_DEFAULTS);
  const clearedResize = useBayResize("clx-bay-heights-cleared", CLEARED_DEFAULTS);
  const pushbackResize = useBayResize("clx-bay-heights-pushback", PUSHBACK_DEFAULTS);

  const mapToStrip = (strip: AnyStrip, status: string) => (
    <Strip
      key={stripDndId(strip)}
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
            <span className="flex gap-1">
              <button className={CLS_BTN_NEW} onClick={() => setNewOpen(true)}>NEW</button>
              <button className={CLS_BTN_PLANNED} onClick={() => setPlannedOpen(true)}>PLANNED</button>
            </span>
          </div>
          <div className="h-[calc(100%-2.5rem)] bay-scroll-area">
            {otherStrips.map(strip => mapToStrip(strip, "CLR"))}
          </div>
        </div>
        <div style={sasResize.columnStyle} className={col}>
          <div className="bay-col-header justify-between">
            <span className={lockedLabel}>SAS</span>
          </div>
          <div className="h-[var(--bay-h-sas)] bay-scroll-area">
            {sasStrips.map(strip => mapToStrip(strip, "CLR"))}
          </div>
          <div className="bay-col-header bay-col-sep justify-between">
            <BayResizeHandle {...sasResize.handleProps("sas")} />
            <span className={lockedLabel}>NORWEGIAN</span>
          </div>
          <div className="flex-1 bay-scroll-area">
            {norgewianStrips.map(strip => mapToStrip(strip, "CLR"))}
          </div>
        </div>
        <div style={clearedResize.columnStyle} className={col}>
          <div className="bay-col-header justify-between">
            <span className={primaryLabel}>CLEARED</span>
          </div>
          <div className="h-[var(--bay-h-cleared)] bay-scroll-area">
            {cleared.map(strip => mapToStrip(strip, "CLROK"))}
          </div>
          <div className="bay-col-header-teal bay-col-sep justify-between">
            <BayResizeHandle {...clearedResize.handleProps("cleared")} />
            <span className={primaryLabel}>MESSAGES</span>
            <button className={CLS_BTN_MISSED} onClick={() => setComposeOpen(true)}>FREE TEXT</button>
          </div>
          <div className={`flex-1 ${scrollAreaRaw}`}>
            {messages.map(msg => (
              <MessageStrip key={msg.id} msg={msg} />
            ))}
          </div>
          <MessageComposeDialog open={composeOpen} onClose={() => setComposeOpen(false)} />
        </div>
        <div style={pushbackResize.columnStyle} className={col}>
          <div className="bay-col-header-light justify-between">
            <span className={activeLabel}>PUSHBACK</span>
          </div>
          <AutoAlignedBay className="h-[var(--bay-h-pushback)] bay-scroll-area-bottom" dependencyKey={`pushback:${pushback.length}`}>
            {pushback.map(strip => mapToHalfStrip(strip))}
          </AutoAlignedBay>
          <div className="bay-col-header-light bay-col-sep justify-between">
            <BayResizeHandle {...pushbackResize.handleProps("pushback")} />
            <span className={activeLabel}>TWY DEP</span>
          </div>
          <AutoAlignedBay className="flex-1 bay-scroll-area-bottom" dependencyKey={`taxidep:${taxidep.length}`}>
            {taxidep.map(strip => mapToHalfStrip(strip))}
          </AutoAlignedBay>
        </div>
      </div>

      <NewIfrDialog open={newOpen} onOpenChange={setNewOpen} />
      <PlannedDialog open={plannedOpen} onOpenChange={setPlannedOpen} />
    </>
  );
}
