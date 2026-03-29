import { useState } from "react";
import Time from "@/components/Time";
import MRKBTN from "./MRKBTN";
import MUTEBTN from "./MUTEBTN";
import TRFBRN from "./TRFBRN";
import REQBTN from "./REQBTN";
import ATIS from "./ATIS";
import CDMSIM from "./CDMSIM";
import RunwayStsDialog, { type RunwayStatus } from "./RunwayStsDialog";
import MetarHelper from "@/components/MetarHelper";
import { useAudioSettings } from "@/hooks/useAudioSettings";
import { useAtisCode, useMetar, useRunwaySetup, useSelectedCallsign, useSelectStrip, useWebSocketStore, useStrip } from "@/store/store-hooks";
import { CLS_CMDBTN } from "@/components/strip/shared";
import { Bay } from "@/api/models";

// Bar height matches strip height (4.72vh). Inner elements: calc(4.72vh - 14px) + 7px top/bottom margin.
const CLS_BAR = "h-[4.72vh] w-screen bg-bay-commandbar flex justify-between text-white items-center border-y-2 border-bay-border";

// Inner value boxes — same margin rhythm as CLS_CMDBTN
// Font sizes derived from SVG (2560px base): large values 36px→1.41vw, labels 24px→0.94vw
const CLS_VAL_WHITE = "bg-bay-light text-black text-[1.41vw] font-bold h-[calc(4.72vh-14px)] my-[7px] flex items-center justify-center";
const CLS_VAL_DARK  = "bg-bay-dark text-white  text-[1.41vw] font-bold h-[calc(4.72vh-14px)] my-[7px] flex items-center justify-center";
const CLS_LABEL     = "text-[0.94vw] font-bold text-bay-light px-3";

const SCOPE_LABELS: Record<string, string> = {
  "CLX":  "CLR DEL",
  "AAAD": "AA + AD",
  "AD":   "APRON DEP",
  "EST":  "SEQ PLN",
  "GEGW": "GE + GW",
  "TWTE": "TE + TW",
};

const EKCH_SCOPES = [
  { label: "CLR DEL",    layout: "CLX" },
  { label: "SEQ PLN",    layout: "EST" },
  { label: "APRON DEP",  layout: "AD" },
  { label: "APRON ARR",  layout: "AA" },
  { label: "AA + AD",    layout: "AAAD" },
  { label: "GE + GW",    layout: "GEGW" },
  { label: "TE + TW",    layout: "TWTE" },
];

// EKCH runway pairs with vw widths derived from SVG (canvas: 2560px).
// 140/2560 = 5.47vw, 118/2560 = 4.61vw
const RUNWAY_PAIRS: { pair: string; vw: string }[] = [
  { pair: "04L-22R", vw: "5.47vw" },
  { pair: "04R-22L", vw: "5.47vw" },
  { pair: "12-30",   vw: "4.61vw" },
];

const STATUS_BG: Record<string, string> = {
  OPEN:    "#212121",
  LOW_VIS: "#DD6A12",
  CLOSED:  "#F43A3A",
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
  const { muted, toggleMute } = useAudioSettings();
  const metar = useMetar();
  const atisCode = useAtisCode();
  const currentLayout = useWebSocketStore((state) => state.displayedLayout);
  const setDisplayedLayout = useWebSocketStore((state) => state.setDisplayedLayout);
  const runwaySetup = useRunwaySetup();
  const selectedCallsign = useSelectedCallsign();
  const selectStrip = useSelectStrip();
  const move = useWebSocketStore((state) => state.move);
  const toggleMarked = useWebSocketStore((state) => state.toggleMarked);
  const updateRunwayStatus = useWebSocketStore((state) => state.updateRunwayStatus);
  const strip = useStrip(selectedCallsign ?? "");

  const depRwy = runwaySetup.departure[0] ?? "—";
  const arrRwy = runwaySetup.arrival[0] ?? "—";
  const scopeLabel = SCOPE_LABELS[currentLayout] ?? currentLayout;
  const runwayStatus = runwaySetup.runway_status ?? {};

  const myPosition = useWebSocketStore((state) => state.position);
  const isOwner = !!selectedCallsign && !!myPosition && strip?.owner === myPosition;
  const isMarked = strip?.marked ?? false;

  // Layout chooser dialog (dismissable, triggered from station box)
  const [layoutOpen, setLayoutOpen] = useState(false);

  // Runway status dialog
  const [rwyDlgPair, setRwyDlgPair] = useState<string | null>(null);

  const handleMark = () => {
    if (!selectedCallsign) return;
    toggleMarked(selectedCallsign, !isMarked);
  };

  const handleDelete = () => {
    if (!selectedCallsign || !isOwner) return;
    move(selectedCallsign, Bay.Hidden);
    selectStrip(null);
  };

  const handleLayoutSelect = (l: string) => {
    setDisplayedLayout(l);
    setLayoutOpen(false);
  };

  return (
    <div className={CLS_BAR}>
      {/* ── Left section ─────────────────────────────────── */}
      <div className="flex items-center h-full">

        {/* Scope — green station box, clicks open layout dialog */}
        <button
          onClick={() => setLayoutOpen(true)}
          className="bg-[#1bff16] text-black flex flex-col justify-center items-center mx-2 font-bold h-[calc(4.72vh-14px)] my-[7px] w-[9.73vw] text-center leading-tight outline-none active:brightness-90"
        >
          <span className="text-[0.78vw] font-semibold">{scopeLabel}</span>
          {myPosition && <span className="text-[0.63vw] font-medium">{myPosition}</span>}
        </button>

        {/* DEP runway */}
        <span className={CLS_LABEL}>DEP</span>
        <span className={`${CLS_VAL_WHITE} w-[3.13vw]`}>{depRwy}</span>

        {/* ARR runway */}
        <span className={CLS_LABEL}>ARR</span>
        <span className={`${CLS_VAL_WHITE} w-[3.13vw]`}>{arrRwy}</span>

        {/* QNH */}
        <span className={CLS_LABEL}>QNH</span>
        <span className={`${CLS_VAL_DARK} w-[3.75vw]`}>
          <MetarHelper metar={metar} style="qnh" unit="hPa" />
        </span>

        {/* ATIS button — width overridden in ATIS.tsx */}
        <div className="ml-[5px]">
          <ATIS />
        </div>

        {/* Wind: ATIS code + compact value */}
        <span className={`${CLS_VAL_WHITE} w-[3.36vw] mx-1`}>{atisCode || "—"}</span>
        <span className={`${CLS_VAL_WHITE} w-[5.55vw] px-2 !text-[0.94vw]`}>{parseWindCompact(metar)}</span>
      </div>

      {/* ── Center: runway pair status buttons ────────────── */}
      <div className="flex items-center h-full gap-[5px]">
        {RUNWAY_PAIRS.map(({ pair, vw }) => {
          const status = runwayStatus[pair];
          const bg = (status && STATUS_BG[status]) ?? "#212121";
          return (
            <button
              key={pair}
              onClick={() => setRwyDlgPair(pair)}
              style={{ backgroundColor: bg, width: vw }}
              className="text-white text-[0.94vw] font-bold h-[calc(4.72vh-14px)] my-[7px] flex items-center justify-center shadow-[inset_2px_0_0_var(--color-bay-shadow),_inset_-2px_0_0_var(--color-bay-shadow),_inset_0_2px_0_var(--color-bay-shadow),_inset_0_-2px_0_var(--color-bay-shadow)] outline-none"
            >
              {pair}
            </button>
          );
        })}
      </div>

      {/* ── Right section ────────────────────────────────── */}
      <div className="flex items-center h-full gap-[5px]">
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
        <MUTEBTN muted={muted} onClick={toggleMute} />
        <div className="bg-bay-light text-black h-[calc(4.72vh-14px)] my-[7px] w-[5.08vw] ml-[5px] mr-3 flex items-center justify-center text-[0.75vw] font-bold shadow-[inset_2px_0_0_var(--color-bay-shadow),_inset_0_2px_0_var(--color-bay-shadow)]">
          <Time />
        </div>
      </div>

      {/* ── Layout chooser popup (anchored just above commandbar) ───── */}
      {layoutOpen && (
        <>
          {/* Transparent backdrop — click to close */}
          <div className="fixed inset-0 z-40" onClick={() => setLayoutOpen(false)} />

          {/* Popup panel — matches Scope selector.svg (2512×254 design, 2560px vw base, 2160px vh base) */}
          <div
            className="fixed z-50"
            style={{
              bottom: "calc(4.72vh + 0.5vh)",
              left: "1vw",
              right: "1vw",
              height: "11.76vh",
              background: "#B3B3B3",
              border: "1px solid black",
            }}
          >
            {/* Inner border inset (SVG: 15.5px sides / 16.5px top-bottom at 2512×254) */}
            <div
              className="absolute flex items-center"
              style={{
                inset: "0.76vh 0.60vw",
                border: "1px solid black",
              }}
            >
              {/* Scope buttons */}
              <div className="flex items-center gap-[1.56vw] pl-[0.66vw] h-full py-[0.74vh]">
                {EKCH_SCOPES.map((scope) => (
                  <button
                    key={scope.layout}
                    onClick={() => handleLayoutSelect(scope.layout)}
                    style={{
                      width: "9.50vw",
                      background: currentLayout === scope.layout ? "#1BFF16" : "#D6D6D6",
                      color: "black",
                      fontSize: "0.96vw",
                      fontWeight: 500,
                      height: "100%",
                      boxShadow: "2px 4px 4px rgba(0,0,0,0.25)",
                      fontFamily: "Rubik, sans-serif",
                    }}
                  >
                    {scope.label}
                  </button>
                ))}
              </div>

              {/* OK button — right-aligned, dark */}
              <button
                onClick={() => setLayoutOpen(false)}
                style={{
                  marginLeft: "auto",
                  marginRight: "0.66vw",
                  width: "7.42vw",
                  height: "calc(100% - 1.48vh)",
                  background: "#3F3F3F",
                  color: "white",
                  fontSize: "1.25vw",
                  fontWeight: 600,
                  boxShadow: "2px 4px 4px rgba(0,0,0,0.25)",
                  fontFamily: "Rubik, sans-serif",
                }}
              >
                OK
              </button>
            </div>
          </div>
        </>
      )}

      {/* ── Runway status dialog ───────────────────────────── */}
      {rwyDlgPair && (
        <RunwayStsDialog
          pair={rwyDlgPair}
          open={rwyDlgPair !== null}
          onClose={() => setRwyDlgPair(null)}
          onSelect={(status: RunwayStatus) => updateRunwayStatus(rwyDlgPair, status)}
        />
      )}
    </div>
  );
}
