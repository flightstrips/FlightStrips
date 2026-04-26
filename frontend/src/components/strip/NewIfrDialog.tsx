import { useState } from "react";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import * as VisuallyHidden from "@radix-ui/react-visually-hidden";
import { useStrips, useWebSocketStore, useAvailableSids, useRunwaySetup } from "@/store/store-hooks";
import { SidSelectDialog } from "@/components/strip/SidSelectDialog";
import { HdgSelectDialog } from "@/components/strip/HdgSelectDialog";
import { AltSelectDialog } from "@/components/strip/AltSelectDialog";
import { RunwayDialog } from "@/components/strip/RunwayDialog";
import { scalePx } from "@/lib/viewportScale";

// Exactly mirrors FlightPlanDialog constants
const FONT_FAMILY      = "Arial";
const FONT_SIZE_FIELD  = scalePx(20);
const FONT_SIZE_LABEL  = scalePx(16);
const FONT_SIZE_BUTTON = scalePx(24);
const FIELD_HEIGHT     = scalePx(50);
const TEXTAREA_HEIGHT  = scalePx(80);
const DIALOG_WIDTH     = scalePx(1000);
const DIALOG_HEIGHT    = scalePx(925);
const CONTENT_WIDTH    = scalePx(835);
const PANEL_PADDING    = scalePx(30);
const DIALOG_PADDING   = scalePx(25);
const LABEL_OFFSET     = scalePx(11);
const FIELD_GAP        = scalePx(5);
const CLS_DIALOG       = "bg-[#d4d4d4] rounded-none flex flex-col gap-0";
const CLS_DIALOG_LABEL = "absolute bg-[#d4d4d4] text-black font-bold";
const CLS_DISABLED     = "border border-black rounded-none bg-[#b3b3b3] text-black font-bold text-center disabled:opacity-60";
const CLS_EDITABLE     = "border border-black rounded-none bg-[#ededed] text-black font-bold text-center focus-visible:outline-none focus-visible:ring-0";
const CLS_EDITABLE_BTN = "border border-black rounded-none bg-[#ededed] text-black font-bold text-center";
const CLS_TEXTAREA     = "border border-black rounded-none bg-[#ededed] text-black font-normal text-center break-words resize-none w-full focus:outline-none";
const COLOR_DARK_BTN   = "#3F3F3F";

// All physical runways — same constant as RunwayDialog
const RUNWAYS = ["04R", "04L", "12", "22R", "22L", "30"];

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialCallsign?: string;
}

export function NewIfrDialog({ open, onOpenChange, initialCallsign = "" }: Props) {
  const strips          = useStrips();
  const availableSids   = useAvailableSids();
  const runwaySetup     = useRunwaySetup();
  const createManualFPL = useWebSocketStore(s => s.createManualFPL);

  const [prevOpen, setPrevOpen]           = useState(false);
  const [callsign, setCallsign]           = useState(initialCallsign);
  const [callsignError, setCallsignError] = useState<string | null>(null);
  const [ades, setAdes]                   = useState("");
  const [sid, setSid]                     = useState("");
  const [ssr, setSsr]                     = useState("");
  const [eobt, setEobt]                   = useState("");
  const [aircraftType, setAircraftType]   = useState("");
  const [fl, setFl]                       = useState("");
  const [route, setRoute]                 = useState("");
  const [stand, setStand]                 = useState("");
  const [rwyDep, setRwyDep]               = useState(RUNWAYS[0]);
  const [hdg, setHdg]                     = useState<number | undefined>(undefined);
  const [alt, setAlt]                     = useState<number | undefined>(undefined);

  // Sub-dialog open states
  const [sidOpen, setSidOpen] = useState(false);
  const [rwyOpen, setRwyOpen] = useState(false);
  const [hdgOpen, setHdgOpen] = useState(false);
  const [altOpen, setAltOpen] = useState(false);

  if (open !== prevOpen) {
    setPrevOpen(open);
    if (open) {
      const cs = initialCallsign.toUpperCase();
      setCallsign(cs);
      setCallsignError(null);
      setAdes(""); setSid(""); setSsr(""); setEobt("");
      setAircraftType(""); setFl(""); setRoute(""); setStand("");
      setRwyDep(runwaySetup.departure[0] ?? RUNWAYS[0]); setHdg(undefined); setAlt(undefined);
      if (cs) populateFromStrip(cs);
    }
  }

  function populateFromStrip(cs: string) {
    const strip = strips.find(s => s.callsign.toUpperCase() === cs.toUpperCase());
    if (!strip) { setCallsignError("Callsign not connected"); return; }
    setCallsignError(null);
    if (strip.destination)        setAdes(strip.destination);
    if (strip.sid)                setSid(strip.sid);
    if (strip.assigned_squawk)    setSsr(strip.assigned_squawk);
    if (strip.eobt)               setEobt(strip.eobt);
    if (strip.aircraft_type)      setAircraftType(strip.aircraft_type);
    if (strip.requested_altitude) setFl(String(Math.round(strip.requested_altitude / 100)));
    if (strip.route)              setRoute(strip.route);
    if (strip.stand)              setStand(strip.stand);
    if (strip.runway)             setRwyDep(strip.runway);
    if (strip.heading)            setHdg(strip.heading);
    if (strip.cleared_altitude)   setAlt(strip.cleared_altitude);
  }

  function handleCallsignBlur() {
    if (!callsign.trim()) { setCallsignError(null); return; }
    populateFromStrip(callsign.trim());
  }

  function handleOk() {
    if (callsignError || !callsign.trim()) return;
    createManualFPL(
      callsign.trim().toUpperCase(),
      ades.trim().toUpperCase(),
      sid.trim().toUpperCase(),
      ssr.trim(),
      eobt.trim(),
      aircraftType.trim().toUpperCase(),
      fl.trim(),
      route.trim().toUpperCase(),
      stand.trim().toUpperCase(),
      rwyDep,
    );
    onOpenChange(false);
  }

  const canSubmit = !callsignError && callsign.trim().length > 0;
  const F = { fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD, height: FIELD_HEIGHT };
  const fieldStyle = (width: number) => ({ width: scalePx(width), ...F });
  const rowStyle = { width: CONTENT_WIDTH, gap: FIELD_GAP };
  const groupStyle = { gap: FIELD_GAP };
  const footerButtonStyle = {
    width: scalePx(125),
    height: scalePx(70),
    fontFamily: FONT_FAMILY,
    fontWeight: "bold" as const,
    fontSize: FONT_SIZE_BUTTON,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    cursor: "pointer",
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className={CLS_DIALOG}
        style={{ width: DIALOG_WIDTH, maxWidth: DIALOG_WIDTH, height: DIALOG_HEIGHT, maxHeight: DIALOG_HEIGHT, padding: DIALOG_PADDING }}
      >
        <VisuallyHidden.Root>
          <DialogTitle>New IFR flight plan</DialogTitle>
        </VisuallyHidden.Root>

        <div
          className="relative border-2 border-black flex flex-col items-center flex-1 min-h-0"
          style={{ gap: PANEL_PADDING, paddingTop: PANEL_PADDING, paddingBottom: PANEL_PADDING, color: "black" }}
        >
          <span
            className={CLS_DIALOG_LABEL}
            style={{ top: `calc(-1 * ${LABEL_OFFSET})`, left: "50%", transform: "translateX(-50%)", whiteSpace: "nowrap", paddingInline: scalePx(5) }}
          >
            NEW IFR
          </span>

          {/* Row 1: C/S | ADES | RNAV | SID | SSR | TTOT | CTOT */}
          <div className="flex" style={rowStyle}>
            <div className="grid items-center" style={groupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>C/S</Label>
              <input
                className={CLS_EDITABLE}
                style={fieldStyle(180)}
                value={callsign}
                onChange={e => setCallsign(e.target.value.toUpperCase())}
                onBlur={handleCallsignBlur}
                autoFocus
              />
            </div>
            <div className="grid items-center" style={groupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>ADES</Label>
              <input className={CLS_EDITABLE} style={fieldStyle(100)} value={ades} onChange={e => setAdes(e.target.value.toUpperCase())} />
            </div>
            <div className="grid items-center" style={groupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>RNAV</Label>
              <Input disabled className={CLS_DISABLED} style={fieldStyle(75)} />
            </div>
            <div className="grid items-center" style={groupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>SID</Label>
              <button
                type="button"
                className={CLS_EDITABLE_BTN}
                style={fieldStyle(150)}
                onClick={() => setSidOpen(true)}
              >
                {sid}
              </button>
                <SidSelectDialog
                  open={sidOpen}
                  onOpenChange={setSidOpen}
                  value={sid}
                  onSelect={setSid}
                  onErase={() => {
                    setSid("");
                    setSidOpen(false);
                  }}
                  sids={availableSids.length > 0
                    ? availableSids.filter(s => s.runway === rwyDep).map(s => s.name)
                    : undefined}
                />
            </div>
            <div className="grid items-center" style={groupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>SSR</Label>
              <input className={CLS_EDITABLE} style={fieldStyle(100)} value={ssr} onChange={e => setSsr(e.target.value)} />
            </div>
            <div className="grid items-center" style={groupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>TTOT</Label>
              <Input disabled className={CLS_DISABLED} style={fieldStyle(100)} />
            </div>
            <div className="grid items-center" style={groupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>CTOT</Label>
              <Input disabled className={CLS_DISABLED} style={fieldStyle(100)} />
            </div>
          </div>

          {/* Row 2: EOBT | TOBT | TSAT | RWY  —  REA */}
          <div className="flex justify-between" style={rowStyle}>
            <div className="flex" style={{ gap: FIELD_GAP }}>
              <div className="grid items-center" style={groupStyle}>
                <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>EOBT</Label>
                <input className={CLS_EDITABLE} style={fieldStyle(100)} value={eobt} onChange={e => setEobt(e.target.value)} maxLength={4} />
              </div>
              <div className="grid items-center" style={groupStyle}>
                <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>TOBT</Label>
                <Input disabled className={CLS_DISABLED} style={fieldStyle(100)} />
              </div>
              <div className="grid items-center" style={groupStyle}>
                <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>TSAT</Label>
                <Input disabled className={CLS_DISABLED} style={fieldStyle(100)} />
              </div>
              <div className="grid items-center" style={groupStyle}>
                <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>RWY</Label>
                <button
                  type="button"
                  className={CLS_EDITABLE_BTN}
                  style={fieldStyle(150)}
                  onClick={() => setRwyOpen(true)}
                >
                  {rwyDep}
                </button>
                <RunwayDialog
                  mode="SELECT"
                  open={rwyOpen}
                  onOpenChange={setRwyOpen}
                  currentRunway={rwyDep}
                  onSelect={setRwyDep}
                />
              </div>
            </div>
            <div className="grid items-center" style={groupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>REA</Label>
              <Input disabled className={CLS_DISABLED} style={fieldStyle(100)} />
            </div>
          </div>

          {/* Row 3: TYPE | FL | SPEED | STS */}
          <div className="flex" style={rowStyle}>
            <div className="grid items-center" style={groupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>TYPE</Label>
              <input className={CLS_EDITABLE} style={fieldStyle(200)} value={aircraftType} onChange={e => setAircraftType(e.target.value.toUpperCase())} />
            </div>
            <div className="grid items-center" style={groupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>FL</Label>
              <input className={CLS_EDITABLE} style={fieldStyle(100)} value={fl} onChange={e => setFl(e.target.value)} />
            </div>
            <div className="grid items-center" style={groupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>SPEED</Label>
              <Input disabled className={CLS_DISABLED} style={fieldStyle(100)} />
            </div>
            <div className="grid items-center" style={groupStyle}>
              <Label className="font-light text-center" style={{ fontSize: FONT_SIZE_LABEL }}>STS</Label>
              <Input disabled className={CLS_DISABLED} style={fieldStyle(420)} />
            </div>
          </div>

          {/* Row 4: ROUTE */}
          <div className="flex flex-col" style={{ width: CONTENT_WIDTH, gap: FIELD_GAP }}>
            <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>ROUTE</Label>
            <textarea
              className={CLS_TEXTAREA}
              style={{ height: TEXTAREA_HEIGHT, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              value={route}
              onChange={e => setRoute(e.target.value.toUpperCase())}
            />
          </div>

          {/* Row 5: COOPANS REMARKS */}
          <div className="flex flex-col" style={{ width: CONTENT_WIDTH, gap: FIELD_GAP }}>
            <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>COOPANS REMARKS</Label>
            <Input disabled className={`${CLS_DISABLED} w-full`} style={{ ...F }} />
          </div>

          {/* Row 6: NITOS REMARKS | IATA TYPE */}
          <div className="flex" style={rowStyle}>
            <div className="grid items-center" style={groupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>NITOS REMARKS</Label>
              <Input
                disabled
                className={CLS_DISABLED}
                style={{ ...fieldStyle(700), color: callsignError ? "#cc0000" : undefined }}
                value={callsignError ?? ""}
              />
            </div>
            <div className="grid items-center" style={groupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>IATA TYPE</Label>
              <Input disabled className={CLS_DISABLED} style={fieldStyle(130)} />
            </div>
          </div>

          {/* Row 7: CLIMB GR | HDG | ALT | DE-ICE | REG | STAND */}
          <div className="flex justify-between" style={{ width: CONTENT_WIDTH }}>
            <div className="grid items-center" style={groupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>CLIMB GR.</Label>
              <Input disabled className={CLS_DISABLED} style={fieldStyle(125)} />
            </div>
            <div className="grid items-center" style={groupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>HDG</Label>
              <button
                type="button"
                className={CLS_EDITABLE_BTN}
                style={fieldStyle(125)}
                onClick={() => setHdgOpen(true)}
              >
                {hdg != null ? hdg.toString().padStart(3, "0") : ""}
              </button>
              <HdgSelectDialog
                open={hdgOpen}
                onOpenChange={setHdgOpen}
                value={hdg}
                onSelect={setHdg}
              />
            </div>
            <div className="grid items-center" style={groupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>ALT</Label>
              <button
                type="button"
                className={CLS_EDITABLE_BTN}
                style={fieldStyle(125)}
                onClick={() => setAltOpen(true)}
              >
                {alt != null ? String(alt) : ""}
              </button>
              <AltSelectDialog
                open={altOpen}
                onOpenChange={setAltOpen}
                value={alt}
                onSelect={setAlt}
              />
            </div>
            <div className="grid items-center" style={groupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>DE-ICE</Label>
              <Input disabled className={CLS_DISABLED} style={fieldStyle(125)} />
            </div>
            <div className="grid items-center" style={groupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>REG</Label>
              <Input disabled className={CLS_DISABLED} style={fieldStyle(125)} />
            </div>
            <div className="grid items-center" style={groupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>STAND</Label>
              <input
                className={CLS_EDITABLE}
                style={fieldStyle(125)}
                value={stand}
                onChange={e => setStand(e.target.value.toUpperCase())}
              />
            </div>
          </div>
        </div>

        {/* Footer: ESC | OK */}
        <div className="flex flex-row items-center justify-between" style={{ paddingTop: scalePx(12) }}>
          <button
            onClick={() => onOpenChange(false)}
            style={{
              ...footerButtonStyle,
              backgroundColor: COLOR_DARK_BTN, color: "white",
            }}
          >
            ESC
          </button>
          <button
            onClick={handleOk}
            disabled={!canSubmit}
            style={{
              ...footerButtonStyle,
              backgroundColor: canSubmit ? COLOR_DARK_BTN : "#888",
              color: canSubmit ? "white" : "#bbb",
              cursor: canSubmit ? "pointer" : "not-allowed",
            }}
          >
            OK
          </button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
