import Time from "@/components/Time";
import MRKBTN from "./MRKBTN";
import TRFBRN from "./TRFBRN";
import REQBTN from "./REQBTN";
import ATIS from "./ATIS";
import HOMEBTN from "./HOMEBTN";
import CDMSIM from "./CDMSIM";
import MetarHelper from "@/components/MetarHelper";
import { useAtisCode, useMetar, useRunwaySetup, useSelectedCallsign, useSelectStrip, useWebSocketStore, useStrip } from "@/store/store-hooks";
import { CLS_CMDBTN } from "@/components/strip/shared";

// CommandBar-specific class constants
const CLS_BAR        = "h-16 w-screen bg-[#3b3b3b] flex justify-between text-white";
const CLS_SCOPE_LBL  = "bg-[#1bff16] text-black w-32 flex justify-center items-center m-2 font-bold";
const CLS_QNH_DARK   = "bg-[#212121] w-18 p-2"; // dark display for QNH value
const CLS_TIME_BOX   = "w-32 bg-[#646464] flex items-center justify-center h-6/8 border-2";
import { Bay } from "@/api/models";

const SCOPE_LABELS: Record<string, string> = {
  "CLX": "CLR DEL",
  "AAAD": "AA + AD",
  "GEGW": "GE / GW",
  "TWTE": "TW / TE",
};

export default function CommandBar() {
  const metar = useMetar();
  const atisCode = useAtisCode();
  const layout = useWebSocketStore((state) => state.layout);
  const runwaySetup = useRunwaySetup();
  const selectedCallsign = useSelectedCallsign();
  const selectStrip = useSelectStrip();
  const move = useWebSocketStore((state) => state.move);
  const toggleMarked = useWebSocketStore((state) => state.toggleMarked);
  const strip = useStrip(selectedCallsign ?? "");

  const depRwy = runwaySetup.departure[0] ?? "—";
  const arrRwy = runwaySetup.arrival[0] ?? "—";

  const scopeLabel = SCOPE_LABELS[layout] ?? layout;

  const myPosition = useWebSocketStore((state) => state.position);
  const isOwner = !!selectedCallsign && !!myPosition && strip?.owner === myPosition;

  const isMarked = strip?.marked ?? false;

  const handleMark = () => {
    if (!selectedCallsign) return;
    toggleMarked(selectedCallsign, !isMarked);
  };

  const handleDelete = () => {
    if (!selectedCallsign || !isOwner) return;
    move(selectedCallsign, Bay.Hidden);
    selectStrip(null);
  };

  return (
    <div className={CLS_BAR}>
      <div className="h-full w-full flex">
        <div className={CLS_SCOPE_LBL}>
          {scopeLabel}
        </div>
        <div className="flex w-32 text-2xl font-bold m-2 items-center justify-between">
          <h1>DEP</h1>
          <span className="bg-white text-black w-16 p-2">{depRwy}</span>
        </div>
        <div className="flex w-32 text-2xl font-bold m-2 items-center justify-between">
          <h1>ARR</h1>
          <span className="bg-white text-black w-16 p-2">{arrRwy}</span>
        </div>
        <div className="flex w-fit text-2xl font-bold m-2 items-center justify-between gap-2">
          <h1>QNH</h1>
          <span className={CLS_QNH_DARK}>
            <MetarHelper metar={metar} style="qnh" unit="hPa" />
          </span>
          {atisCode && (
            <span className="bg-[#212121] text-white w-10 p-2 text-center">
              {atisCode}
            </span>
          )}
          <span className="bg-white text-black w-32 p-2 text-center text-xl">
            <MetarHelper metar={metar} style="winds" />
          </span>
        </div>
        <div className="flex w-fit text-2xl font-bold m-2 items-center justify-between">
          <ATIS />
        </div>
      </div>
      <div className="flex items-center justify-center gap-1">
        <HOMEBTN />
        <TRFBRN />
        <MRKBTN isMarked={isMarked} disabled={!selectedCallsign} onClick={handleMark} />
        <REQBTN />
        {import.meta.env.VITE_CDM_SIM === 'true' && <CDMSIM />}
        <button
          disabled={!isOwner}
          className={`${CLS_CMDBTN} ${!isOwner ? "opacity-50 cursor-not-allowed" : ""}`}
          onClick={handleDelete}
        >
          X
        </button>
        <div className={CLS_TIME_BOX}>
          <Time />
        </div>
      </div>
    </div>
  );
}