import { useState } from "react";
import { Bay, isFlight, stripDndId, type AnyStrip } from "@/api/models";
import { Strip } from "@/components/strip/Strip";
import { MessageStrip } from "@/components/strip/MessageStrip";
import { MessageComposeDialog } from "@/components/MessageComposeDialog";
import { NewIfrDialog } from "@/components/strip/NewIfrDialog";
import { PlannedDialog } from "@/components/strip/PlannedDialog";
import { AutoAlignedBay } from "@/components/bays/SortableBay";
import { CLS_BTN, CLS_LABEL } from "@/components/strip/shared";
import { useMessages, useMyPosition } from "@/store/store-hooks";
import {
  useClearedStrips,
  useFinalStrips,
  useNorwegianBayStrips,
  useOtherBayStrips,
  usePushbackStrips,
  useRwyArrStrips,
  useSasBayStrips,
  useTaxiArrStrips,
  useTaxiDepStrips,
} from "@/store/airports/ekch";
import { orderSeqPlnStartup } from "./seqPlnOrder";

const header = "bay-col-header justify-between";
const passiveHeader = "bay-col-header-light justify-between";
const separator = "bay-col-sep";
const passiveLabel = "text-bay-header font-bold text-[0.94vw]";

export default function SEQPLN() {
  const myPosition = useMyPosition();
  const messages = useMessages();
  const [composeOpen, setComposeOpen] = useState(false);
  const [newOpen, setNewOpen] = useState(false);
  const [plannedOpen, setPlannedOpen] = useState(false);

  const final = useFinalStrips().sort((a, b) => b.sequence - a.sequence);
  const rwyArr = useRwyArrStrips().sort((a, b) => b.sequence - a.sequence);
  const twyArr = useTaxiArrStrips().sort((a, b) => b.sequence - a.sequence);
  const startup = orderSeqPlnStartup(useClearedStrips());
  const sas = useSasBayStrips().sort((a, b) => a.sequence - b.sequence);
  const pushback = usePushbackStrips().sort((a, b) => b.sequence - a.sequence);
  const twyDep = useTaxiDepStrips().sort((a, b) => b.sequence - a.sequence);
  const norwegian = useNorwegianBayStrips().sort((a, b) => a.sequence - b.sequence);
  const others = useOtherBayStrips().sort((a, b) => a.sequence - b.sequence);

  const renderStrip = (strip: AnyStrip, status: "CLR" | "CLROK" | "PUSH" | "ARR" | "TAXI-DEP" | "HALF", passive = false) => (
    <div key={stripDndId(strip)} inert={passive} className={passive ? "pointer-events-none" : undefined}>
      <Strip
        strip={strip}
        status={status}
        halfStripVariant={status === "HALF" ? "LOCKED-ARR" : undefined}
        myPosition={myPosition}
        selectable={!passive}
        startupSiTransfer={status === "CLROK" && isFlight(strip) && strip.bay === Bay.Cleared}
      />
    </div>
  );

  return (
    <>
      <div className="bay-page-wrapper">
        <div className="bay-col-flex">
          <div className="bay-col-header-primary justify-between">
            <span className="text-white font-bold text-lg">MESSAGES</span>
            <button className={CLS_BTN} onClick={() => setComposeOpen(true)}>FREE TEXT</button>
          </div>
          <div className="h-[23%] bay-scroll-area">
            {messages.map(message => <MessageStrip key={message.id} msg={message} />)}
          </div>

          <div className={`${passiveHeader} ${separator}`}><span className={passiveLabel}>FINAL</span></div>
          <AutoAlignedBay className="h-[22%] bay-scroll-area-bottom" dependencyKey={`final:${final.length}`}>
            {final.map(strip => renderStrip(strip, "HALF", true))}
          </AutoAlignedBay>

          <div className={`${passiveHeader} ${separator}`}><span className={passiveLabel}>RWY ARR</span></div>
          <AutoAlignedBay className="h-[10%] bay-scroll-area-bottom" dependencyKey={`rwy:${rwyArr.length}`}>
            {rwyArr.map(strip => renderStrip(strip, "ARR", true))}
          </AutoAlignedBay>

          <div className={`${passiveHeader} ${separator}`}><span className={passiveLabel}>TWY ARR</span></div>
          <AutoAlignedBay className="flex-1 bay-scroll-area-bottom" dependencyKey={`twy:${twyArr.length}`}>
            {twyArr.map(strip => renderStrip(strip, "ARR", true))}
          </AutoAlignedBay>
        </div>

        <div className="bay-col-flex">
          <div className={header}><span className={CLS_LABEL}>STARTUP</span></div>
          <AutoAlignedBay className="flex-1 bay-scroll-area-bottom" dependencyKey={`startup:${startup.map(strip => stripDndId(strip)).join(",")}`}>
            {startup.map(strip => renderStrip(strip, "CLROK"))}
          </AutoAlignedBay>
        </div>

        <div className="bay-col-flex">
          <div className={header}><span className={CLS_LABEL}>SAS</span></div>
          <div className="h-[25%] bay-scroll-area">{sas.map(strip => renderStrip(strip, "CLR"))}</div>

          <div className={`${header} ${separator}`}><span className={CLS_LABEL}>PUSHBACK</span></div>
          <AutoAlignedBay className="h-[18%] bay-scroll-area-bottom" dependencyKey={`push:${pushback.length}`}>
            {pushback.map(strip => renderStrip(strip, "PUSH"))}
          </AutoAlignedBay>

          <div className={`${passiveHeader} ${separator}`}><span className={passiveLabel}>TWY DEP</span></div>
          <AutoAlignedBay className="flex-1 bay-scroll-area-bottom" dependencyKey={`taxi:${twyDep.length}`}>
            {twyDep.map(strip => renderStrip(strip, "TAXI-DEP"))}
          </AutoAlignedBay>
        </div>

        <div className="bay-col-flex">
          <div className={header}>
            <span className={CLS_LABEL}>NORWEGIAN</span>
            <span className="flex gap-1">
              <button className={CLS_BTN} onClick={() => setNewOpen(true)}>NEW</button>
              <button className={CLS_BTN} onClick={() => setPlannedOpen(true)}>PLANNED</button>
            </span>
          </div>
          <div className="h-[30%] bay-scroll-area">{norwegian.map(strip => renderStrip(strip, "CLR"))}</div>
          <div className={`${header} ${separator}`}><span className={CLS_LABEL}>OTHERS</span></div>
          <div className="flex-1 bay-scroll-area">{others.map(strip => renderStrip(strip, "CLR"))}</div>
        </div>
      </div>

      <MessageComposeDialog open={composeOpen} onClose={() => setComposeOpen(false)} />
      <NewIfrDialog open={newOpen} onOpenChange={setNewOpen} />
      <PlannedDialog open={plannedOpen} onOpenChange={setPlannedOpen} />
    </>
  );
}
