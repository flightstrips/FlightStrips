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
import { Bay } from "@/api/models";

// Bar: 60px total. Inner elements: 46px height + 7px top/bottom margin = 60px.
const CLS_BAR = "h-[60px] w-screen bg-[#3c3c3c] flex justify-between text-white items-center";

// Inner value boxes — same margin rhythm as CLS_CMDBTN
const CLS_VAL_WHITE = "bg-[#e4e4e4] text-black text-2xl font-bold h-[46px] my-[7px] flex items-center justify-center";
const CLS_VAL_DARK  = "bg-[#212121] text-white  text-2xl font-bold h-[46px] my-[7px] flex items-center justify-center";
const CLS_LABEL     = "text-2xl font-bold text-[#e4e4e4] px-3";

const SCOPE_LABELS: Record<string, string> = {
  "CLX":  "CLR DEL",
  "AAAD": "AA + AD",
  "GEGW": "GE / GW",
  "TWTE": "TW / TE",
};

function parseWindCompact(metar: string | null): string {
  if (!metar) return "— / —";
  if (/\b00000KT\b/.test(metar)) return "000 / 00";
  const vrb = metar.match(/\bVRB(\d{2})(?:G\d{2})?KT\b/);
  if (vrb) return `VRB / ${vrb[1]}`;
  const w = metar.match(/\b(\d{3})(\d{2})(?:G\d{2})?KT\b/);
  if (w) return `${w[1]} / ${w[2]}`;
  return "— / —";
}

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
      {/* ── Left section ─────────────────────────────────── */}
      <div className="flex items-center h-full">

        {/* Scope — green station box */}
        <div className="bg-[#1bff16] text-black flex flex-col justify-center items-center mx-2 font-bold h-[46px] my-[7px] min-w-[190px] px-3 text-center leading-tight">
          <span className="text-sm font-semibold">{scopeLabel}</span>
          {myPosition && <span className="text-xs font-medium">{myPosition}</span>}
        </div>

        {/* DEP runway */}
        <span className={CLS_LABEL}>DEP</span>
        <span className={`${CLS_VAL_WHITE} w-[92px]`}>{depRwy}</span>

        {/* ARR runway */}
        <span className={CLS_LABEL}>ARR</span>
        <span className={`${CLS_VAL_WHITE} w-[92px]`}>{arrRwy}</span>

        {/* QNH */}
        <span className={CLS_LABEL}>QNH</span>
        <span className={`${CLS_VAL_DARK} min-w-[92px] px-3`}>
          <MetarHelper metar={metar} style="qnh" unit="hPa" />
        </span>

        {/* ATIS button — width overridden in ATIS.tsx */}
        <ATIS atisCode={atisCode} />

        {/* Wind: [D] label + compact value */}
        <span className={`${CLS_VAL_WHITE} w-[64px] mx-1`}>D</span>
        <span className={`${CLS_VAL_WHITE} w-[160px] px-2`}>{parseWindCompact(metar)}</span>
      </div>

      {/* ── Right section ────────────────────────────────── */}
      {/* gap-[5px] between buttons; time box gets extra ml-[5px] for double gap */}
      <div className="flex items-center h-full gap-[5px]">
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
        {/* Time — white box, double gap before it */}
        <div className="bg-[#e4e4e4] text-black h-[46px] my-[7px] w-[96px] ml-[5px] mr-3 flex items-center justify-center text-sm font-bold shadow-[inset_2px_0_0_#d3d3d3,_inset_0_2px_0_#d3d3d3]">
          <Time />
        </div>
      </div>
    </div>
  );
}
