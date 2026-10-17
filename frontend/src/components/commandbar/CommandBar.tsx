import { Fragment, useEffect, useState, type CSSProperties } from "react";
import { ChevronDown, ChevronUp, Maximize, Minimize } from "lucide-react";
import { toast } from "sonner";
import Time from "@/components/Time";
import MRKBTN from "./MRKBTN";
import VACSBTN from "./VACSBTN";
import CommandBarSettings from "./CommandBarSettings";
import TRFBRN from "./TRFBRN";
import REQBTN from "./REQBTN";
import ATIS from "./ATIS";
import CDMSIM from "./CDMSIM";
import RunwayStsDialog, { type RunwayStatus } from "./RunwayStsDialog";
import DeleteConfirmDialog from "./DeleteConfirmDialog";
import MetarHelper from "@/components/MetarHelper";
import { useAtisCode, useMarkArmed, useMetar, useRunwaySetup, useSelectedCallsign, useSelectStrip, useSetMarkArmed, useStrips, useWebSocketStore, useStrip } from "@/store/store-hooks";
import { CLS_CMDBTN, CLS_CMD_BEVEL } from "@/components/strip/shared";
import { Bay } from "@/api/models";
import { FRONTEND_VERSION } from "@/lib/app-version";

// Bar height matches strip height (4.72dvh). Inner elements: calc(4.72dvh - 14px) + 7px top/bottom margin.
const CLS_BAR = "h-[4.72dvh] w-screen bg-bay-commandbar flex justify-between text-white items-center border-t-2 border-bay-border shadow-[inset_0_1px_0_rgba(255,255,255,0.22),inset_0_-2px_2px_rgba(0,0,0,0.35)]";

// Inner value boxes — same margin rhythm as CLS_CMDBTN
// Font sizes derived from SVG (2560px base): large values 36px→1.41vw, labels 24px→0.94vw
const CLS_VAL_WHITE = "bg-bay-light text-black text-[1.0575vw] font-bold h-[calc(4.72dvh-14px)] my-[7px] flex items-center justify-center";
const CLS_VAL_DARK  = "bg-bay-dark text-white  text-[1.0575vw] font-bold h-[calc(4.72dvh-14px)] my-[7px] flex items-center justify-center";
const CLS_VAL_ALERT = "bg-[#F43A3A] text-white text-[1.0575vw] font-bold h-[calc(4.72dvh-14px)] my-[7px] flex items-center justify-center";
const CLS_LABEL     = "text-[0.705vw] font-bold text-bay-light px-3";

const SCOPE_LABELS: Record<string, string> = {
  "CLX":  "CLR DEL",
  "AAAD": "AA + AD",
  "AD":   "APRON DEP",
  "EST":  "EST",
  "SEQPLN": "SEQ PLN",
  "AMAN": "AMAN",
  "GEGW": "GE + GW",
  "TWTE": "TE + TW",
  "TWRGND": "TWR + GND",
};

const LOWER_SCOPES = [
  { label: "EST",        layout: "EST" },
  { label: "CLR DEL",    layout: "CLX" },
  { label: "SEQ PLN",    layout: "SEQPLN" },
  { label: "AA + AD",    layout: "AAAD" },
  { label: "GE + GW",    layout: "GEGW" },
  { label: "TE + TW",    layout: "TWTE" },
  { label: "TWR + GND",  layout: "TWRGND" },
];

const UPPER_SCOPES = [
  { label: "AMAN",       layout: "AMAN" },
  { label: "APRON DEP",  layout: "AD" },
  { label: "APRON ARR",  layout: "AA" },
];

const SCOPE_BEVEL_STYLE: CSSProperties = {
  border: "2px solid",
  borderTopColor: "#E4E4E4",
  borderLeftColor: "#E4E4E4",
  borderBottomColor: "#777777",
  borderRightColor: "#777777",
  boxShadow: "inset 1px 1px 1px rgba(255,255,255,0.15), inset -1px -1px 1px rgba(0,0,0,0.15), 1px 2px 2px rgba(0,0,0,0.15)",
};

const SCOPE_BTN_STYLE: CSSProperties = {
  ...SCOPE_BEVEL_STYLE,
  width: "8.25vw",
  height: "100%",
  color: "black",
  fontSize: "0.96vw",
  fontWeight: 500,
  fontFamily: "var(--font-bay)",
};

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
  const metar = useMetar();
  const atisCode = useAtisCode();
  const currentLayout = useWebSocketStore((state) => state.displayedLayout);
  const setDisplayedLayout = useWebSocketStore((state) => state.setDisplayedLayout);
  const resetLayout = useWebSocketStore((state) => state.resetLayout);
  const readOnly = useWebSocketStore((state) => state.readOnly);
  const positionAvailable = useWebSocketStore((state) => state.positionAvailable);
  const runwaySetup = useRunwaySetup();
  const selectedCallsign = useSelectedCallsign();
  const selectStrip = useSelectStrip();
  const strips = useStrips();
  const markArmed = useMarkArmed();
  const setMarkArmed = useSetMarkArmed();
  const move = useWebSocketStore((state) => state.move);
  const toggleMarked = useWebSocketStore((state) => state.toggleMarked);
  const updateRunwayStatus = useWebSocketStore((state) => state.updateRunwayStatus);
  const strip = useStrip(selectedCallsign ?? "");

  const depRwy = runwaySetup.departure[0] ?? "—";
  const arrRwy = runwaySetup.arrival[0] ?? "—";
  const scopeLabel = readOnly && !positionAvailable
    ? "INVALID"
    : (SCOPE_LABELS[currentLayout] ?? currentLayout);
  const runwayStatus = runwaySetup.runway_status ?? {};
  const depRunwayClass = runwaySetup.departure_mismatch ? CLS_VAL_ALERT : CLS_VAL_WHITE;
  const arrRunwayClass = runwaySetup.arrival_mismatch ? CLS_VAL_ALERT : CLS_VAL_WHITE;

  const myPosition = useWebSocketStore((state) => state.position);
  const scopeButtonClass = readOnly ? "bg-[#FFD84D] text-black" : "bg-[#1bff16] text-black";
  const positionLabel = myPosition ? `${myPosition}${readOnly ? " (OBS)" : ""}` : "";
  const isOwner = !!selectedCallsign && !!myPosition && strip?.owner === myPosition;
  const isMarked = strip?.marked ?? false;
  const canMark = !!selectedCallsign || strips.length > 0;

  // Layout chooser dialog (dismissable, triggered from station box)
  const [layoutOpen, setLayoutOpen] = useState(false);
  const [upperScopesOpen, setUpperScopesOpen] = useState(false);
  const [fullscreen, setFullscreen] = useState(() => !!document.fullscreenElement);
  const fullscreenSupported = document.fullscreenEnabled === true;

  useEffect(() => {
    const updateFullscreen = () => setFullscreen(!!document.fullscreenElement);
    document.addEventListener("fullscreenchange", updateFullscreen);
    return () => document.removeEventListener("fullscreenchange", updateFullscreen);
  }, []);

  const handleFullscreen = async () => {
    try {
      if (document.fullscreenElement) {
        await document.exitFullscreen();
      } else {
        await document.documentElement.requestFullscreen();
      }
    } catch (error) {
      toast.error("Could not change fullscreen mode", {
        description: error instanceof Error ? error.message : "The browser rejected the fullscreen request.",
      });
    }
  };

  // Runway status dialog
  const [rwyDlgPair, setRwyDlgPair] = useState<string | null>(null);

  // Delete confirmation dialog
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);

  useEffect(() => {
    if (!canMark && markArmed) {
      setMarkArmed(false);
    }
  }, [canMark, markArmed, setMarkArmed]);

  const handleMark = () => {
    if (selectedCallsign) {
      toggleMarked(selectedCallsign, !isMarked);
      return;
    }

    if (canMark) {
      setMarkArmed(!markArmed);
    }
  };

  const handleDelete = () => {
    if (!selectedCallsign || !isOwner) return;
    setDeleteConfirmOpen(true);
  };

  const handleDeleteConfirm = () => {
    if (!selectedCallsign || !isOwner) return;
    move(selectedCallsign, Bay.Hidden);
    selectStrip(null);
    setDeleteConfirmOpen(false);
  };

  const handleLayoutSelect = (l: string) => {
    setDisplayedLayout(l);
    setLayoutOpen(false);
  };

  const handleReset = () => {
    resetLayout();
    setLayoutOpen(false);
  };

  const handleOpenEST = () => {
    const url = new URL("/app", window.location.origin);
    url.searchParams.set("layout", "EST");
    const newWindow = window.open(url.href, "_blank", "popup");
    if (!newWindow) {
      toast.error("Could not open EST", { description: "Allow popups for FlightStrips and try again." });
      return;
    }
    newWindow.opener = null;
  };

  return (
    <div className={CLS_BAR}>
      {/* ── Left section ─────────────────────────────────── */}
      <div className="flex items-center h-full">

        {/* Scope — green station box, clicks open layout dialog */}
        <button
          onClick={() => setLayoutOpen(true)}
          className={`${scopeButtonClass} flex flex-col justify-center items-center mx-2 h-[calc(4.72dvh-14px)] my-[7px] w-[9.73vw] text-center leading-tight outline-none active:brightness-90 ${CLS_CMD_BEVEL}`}
        >
          <span className="text-[0.585vw] font-normal">{scopeLabel}</span>
          {positionLabel && <span className="text-[0.4725vw] font-normal">{positionLabel}</span>}
        </button>

        {/* DEP runway */}
        <span className={CLS_LABEL}>DEP</span>
        <span className={`${depRunwayClass} w-[3.13vw] !font-normal`}>{depRwy}</span>

        {/* ARR runway */}
        <span className={CLS_LABEL}>ARR</span>
        <span className={`${arrRunwayClass} w-[3.13vw] !font-normal`}>{arrRwy}</span>

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
        <span className={`${CLS_VAL_WHITE} w-[3.36vw] mx-1 !text-[1.321875vw]`}>{atisCode || "—"}</span>
        <span className={`${CLS_VAL_WHITE} w-[5.55vw] px-2 !text-[0.88125vw]`}>{parseWindCompact(metar)}</span>

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
              className="text-white text-[0.88125vw] font-normal h-[calc(4.72dvh-14px)] my-[7px] flex items-center justify-center shadow-[inset_1px_0_0_var(--color-bay-shadow),_inset_-1px_0_0_var(--color-bay-shadow),_inset_0_1px_0_var(--color-bay-shadow),_inset_0_-1px_0_var(--color-bay-shadow)] outline-none"
            >
              {pair}
            </button>
          );
        })}
      </div>

      {/* ── Right section ────────────────────────────────── */}
      <div className="flex items-center h-full gap-[5px]">
        <TRFBRN />
        <MRKBTN isMarked={isMarked} armed={markArmed} disabled={!canMark} onClick={handleMark} />
        <REQBTN />
        {import.meta.env.VITE_CDM_SIM === 'true' && <CDMSIM />}
        <button
          disabled={!isOwner}
          className={`${CLS_CMDBTN} ${!isOwner ? "opacity-50 cursor-not-allowed" : ""}`}
          onClick={handleDelete}
        >
          X
        </button>
        <div className="relative flex items-center">
          <VACSBTN />
        </div>
        <CommandBarSettings />
        <div className="bg-bay-light text-black h-[calc(4.72dvh-14px)] my-[7px] w-[5.08vw] ml-[5px] mr-3 flex items-center justify-center text-[0.5625vw] font-bold shadow-[inset_2px_0_0_var(--color-bay-shadow),_inset_0_2px_0_var(--color-bay-shadow)]">
          <Time />
        </div>
      </div>

      {/* ── Layout chooser popup (anchored just above commandbar) ───── */}
      {layoutOpen && (
        <>
          {/* Transparent backdrop — click to close */}
          <div className="fixed inset-0 z-40" onClick={() => setLayoutOpen(false)} />

          {/* Popup panel — matches Scope selector.svg (2512×254 design, 2560px vw base, 2160px dvh base) */}
          <div
            className="fixed z-50"
            style={{
              bottom: "calc(4.72dvh + 0.5dvh)",
              left: "1vw",
              width: "max-content",
              maxWidth: "98vw",
              height: upperScopesOpen ? "21.5dvh" : "11.76dvh",
              background: "#B3B3B3",
              border: "1px solid black",
            }}
          >
           <div
             className="absolute text-black/70 font-medium"
             style={{
               left: "0.90vw",
               bottom: "0.20dvh",
               zIndex: 1,
               fontSize: "0.58vw",
               fontFamily: "var(--font-bay)",
             }}
           >
             VERSION {FRONTEND_VERSION}
           </div>

           {/* Inner border inset (SVG: 15.5px sides / 16.5px top-bottom at 2512×254) */}
           <div
              className="relative flex items-end"
              style={{
                margin: "0.76dvh 0.60vw",
                height: "calc(100% - 1.52dvh)",
                border: "1px solid black",
              }}
            >
              {/* Fullscreen / level toggle above RESET */}
              <div className="flex flex-col gap-[0.74dvh] ml-[0.66vw] py-[0.74dvh]" style={{ width: "6.6vw", height: "10.24dvh" }}>
                <div className="flex flex-1 min-h-0">
                  <button
                    onClick={handleFullscreen}
                    disabled={!fullscreenSupported}
                    aria-label={fullscreen ? "Exit fullscreen" : "Enter fullscreen"}
                    aria-pressed={fullscreen}
                    title={fullscreenSupported ? (fullscreen ? "Exit fullscreen" : "Enter fullscreen") : "Fullscreen is not supported in this browser"}
                    className="w-1/2 flex items-center justify-center disabled:opacity-50 disabled:cursor-not-allowed"
                    style={{ ...SCOPE_BEVEL_STYLE, background: "#D6D6D6", color: "black" }}
                  >
                    {fullscreen ? <Minimize className="h-[60%] w-auto" /> : <Maximize className="h-[60%] w-auto" />}
                  </button>
                  <button
                    onClick={() => setUpperScopesOpen((open) => !open)}
                    aria-label={upperScopesOpen ? "Hide more scopes" : "Show more scopes"}
                    aria-expanded={upperScopesOpen}
                    className="w-1/2 flex items-center justify-center"
                    style={{ ...SCOPE_BEVEL_STYLE, background: "#D6D6D6", color: "black" }}
                  >
                    {upperScopesOpen ? <ChevronDown className="h-[60%] w-auto" /> : <ChevronUp className="h-[60%] w-auto" />}
                  </button>
                </div>
                <button
                  onClick={handleReset}
                  className="flex-1 min-h-0"
                  style={{ ...SCOPE_BTN_STYLE, background: "#D6D6D6", width: "100%", height: undefined }}
                >
                  RESET
                </button>
              </div>

              {/* Scope buttons */}
              <div className="relative ml-[0.88vw] my-[0.74dvh]" style={{ height: "8.76dvh" }}>
                {upperScopesOpen && (
                  <div
                    className="absolute grid items-center gap-[0.88vw] w-full"
                    style={{
                      bottom: "calc(100% + 0.74dvh)",
                      height: "8.76dvh",
                      gridTemplateColumns: "8.25vw 2px repeat(6, 8.25vw)",
                    }}
                  >
                    <button
                      onClick={() => handleLayoutSelect(UPPER_SCOPES[0].layout)}
                      style={{ ...SCOPE_BTN_STYLE, background: currentLayout === "AMAN" ? "#1BFF16" : "#D6D6D6" }}
                    >
                      {UPPER_SCOPES[0].label}
                    </button>
                    <div role="separator" aria-orientation="vertical" className="h-full border-l-2 border-black/50" />
                    <div className="flex gap-[0.88vw] h-full" style={{ gridColumn: "3 / span 5", width: "max-content", transform: "translateX(calc((8.25vw + 0.88vw) / 2))" }}>
                      <button
                        onClick={handleOpenEST}
                        style={{ ...SCOPE_BTN_STYLE, background: "#D6D6D6" }}
                      >
                        <span>EST<br /><span className="text-[0.8vw]">(NEW WINDOW)</span></span>
                      </button>
                      {UPPER_SCOPES.slice(1).map((scope) => (
                        <button
                          key={scope.layout}
                          onClick={() => handleLayoutSelect(scope.layout)}
                          style={{ ...SCOPE_BTN_STYLE, background: currentLayout === scope.layout ? "#1BFF16" : "#D6D6D6" }}
                        >
                          {scope.label}
                        </button>
                      ))}
                      {["TOWER EAST", "TOWER WEST"].map((label) => (
                        <button
                          key={label}
                          disabled
                          title="This scope is not yet available"
                          className="cursor-not-allowed"
                          style={{ ...SCOPE_BTN_STYLE, background: "#D6D6D6", color: "#999999" }}
                        >
                          {label}
                        </button>
                      ))}
                    </div>
                  </div>
                )}
                  <div className="flex items-center gap-[0.88vw] h-full">
                    {LOWER_SCOPES.map((scope) => (
                      <Fragment key={scope.layout}>
                        <button
                          onClick={() => handleLayoutSelect(scope.layout)}
                          style={{
                            ...SCOPE_BTN_STYLE,
                            background: currentLayout === scope.layout ? "#1BFF16" : "#D6D6D6",
                          }}
                        >
                          {scope.label}
                        </button>
                        {scope.layout === "EST" && (
                          <div role="separator" aria-orientation="vertical" className="h-full border-l-2 border-black/50" />
                        )}
                      </Fragment>
                    ))}
                  </div>
              </div>

              {/* OK button — right-aligned, dark */}
              <button
                onClick={() => setLayoutOpen(false)}
                className="active:brightness-110 active:translate-y-px active:!shadow-[inset_1px_1px_2px_rgba(0,0,0,0.35)]"
                style={{
                  ...SCOPE_BEVEL_STYLE,
                  marginLeft: "0.88vw",
                  marginRight: "0.66vw",
                  width: "7.42vw",
                  height: "8.76dvh",
                  marginBottom: "0.74dvh",
                  background: "#3F3F3F",
                  color: "white",
                  fontSize: "1.25vw",
                  fontWeight: 600,
                  fontFamily: "var(--font-bay)",
                }}
              >
                OK
              </button>
            </div>
          </div>
        </>
      )}

      {/* ── Delete confirmation dialog ────────────────────── */}
      {deleteConfirmOpen && (
        <DeleteConfirmDialog
          onConfirm={handleDeleteConfirm}
          onCancel={() => setDeleteConfirmOpen(false)}
        />
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
