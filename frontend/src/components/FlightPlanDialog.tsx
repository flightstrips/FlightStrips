import React, { useRef, useState } from "react";
import * as VisuallyHidden from "@radix-ui/react-visually-hidden";

import { Bay, type SidInfo } from "@/api/models.ts";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { formatAltitude, getAircraftTypeWithWtc } from "@/lib/utils";
import { ArrStandDialog } from "@/components/strip/ArrStandDialog";
import { AltSelectDialog } from "@/components/strip/AltSelectDialog";
import { HdgSelectDialog } from "@/components/strip/HdgSelectDialog";
import { SidSelectDialog } from "@/components/strip/SidSelectDialog";
import { RunwayDialog } from "@/components/strip/RunwayDialog";
import { RnavSelectDialog } from "@/components/strip/RnavSelectDialog";
import { useAvailableSids, useInitialCflByRunway, useStrip, useTransitionAltitude, useWebSocketStore } from "@/store/store-hooks.ts";
import { scalePx } from "@/lib/viewportScale";
import { buildRnavUpdate, type RnavCapability } from "@/lib/rnav";
import { normalizeCdmTime } from "@/lib/cdmTime";
import { CDM_RED } from "@/lib/cdmColors";
import { getCtotSlotDisplay, getEcfmpNitosRemarks, getMandatoryRouteRestriction, getGroundStopRestriction, getProhibitRestriction, isFlightLevelViolated } from "@/lib/ecfmp";
import { MandatoryRouteDialog } from "@/components/MandatoryRouteDialog";
import { ManualPdcBypassDialog } from "@/components/ManualPdcBypassDialog";

const FONT_FAMILY = "Arial";
const FONT_SIZE_FIELD = scalePx(20);
const FONT_SIZE_LABEL = scalePx(16);
const FONT_SIZE_BUTTON = scalePx(24);
const FIELD_HEIGHT = scalePx(50);
const TEXTAREA_HEIGHT = scalePx(80);
const DIALOG_WIDTH = scalePx(1000);
const CLEARANCE_DIALOG_HEIGHT = scalePx(925);
const VIEW_DIALOG_HEIGHT = scalePx(1015);
const CONTENT_WIDTH = scalePx(835);
const DIALOG_PADDING = scalePx(25);
const PANEL_PADDING = scalePx(30);
const COMPACT_PANEL_PADDING = scalePx(20);
const FIELD_GAP = scalePx(5);
const LABEL_OFFSET = scalePx(11);
const ACTION_BUTTON_WIDTH = scalePx(125);
const ACTION_BUTTON_HEIGHT = scalePx(70);
const CLX_FALLBACK_WIDTH = scalePx(360);
const CLX_FALLBACK_BUTTON_HEIGHT = scalePx(48);
const MIN_FITTED_FIELD_FONT_SIZE = 9;
const NITOS_SINGLE_LINE_FIT_LENGTH = 70;
const NITOS_MULTILINE_LENGTH = 105;
const NITOS_REMARKS_INLINE_PADDING = scalePx(8);
const SPOKEN_CS_FIT_LENGTH = 8;

// Tailwind class constants (hex must be literal strings for JIT)
const CLS_DIALOG            = "bg-[#d4d4d4] rounded-none flex flex-col gap-0";
const CLS_DIALOG_LABEL      = "absolute bg-[#d4d4d4] text-black font-bold";
const CLS_BTN_DISABLED      = "border border-black rounded-none bg-[#b3b3b3] text-black font-bold text-center disabled:opacity-60";
const CLS_BTN_DISABLED_LEFT = "border border-r-0 border-black rounded-none bg-[#b3b3b3] text-black font-bold text-center disabled:opacity-60";
const CLS_BTN_DISABLED_NRM  = "border border-black rounded-none bg-[#b3b3b3] text-black font-bold text-center disabled:opacity-60";
const CLS_BTN_EDITABLE      = "border border-black rounded-none bg-[#ededed] text-black font-bold text-center";
const CLS_BTN_EDITABLE_LOCK = "border border-black rounded-none bg-[#ededed] text-black font-bold disabled:opacity-100 text-center select-none hover:bg-[#ededed]";
const CLS_NITOS_REMARKS     = "resize-none overflow-hidden border border-black rounded-none bg-[#b3b3b3] text-black font-bold text-center disabled:opacity-60";
const CLS_TEXTAREA_EDITABLE = "border border-black rounded-none bg-[#ededed] text-black font-normal text-center break-words resize-none w-full";
const CLS_CLX_DIALOG        = "rounded-none border border-black bg-[#B3B3B3] text-black";
const CLS_CLX_PANEL         = "border border-black bg-[#D6D6D6]";
// Style-prop color constants (used in CSSProperties, not Tailwind)
const COLOR_DARK_BTN        = "#3F3F3F"; // dark ESC/CLD button
const COLOR_REVERT_BTN      = "#FFFB03"; // yellow revert-to-voice button
const COLOR_CLX_ERROR       = "#FF6D4C";
const COLOR_ECFMP_YELLOW    = "#FFD700"; // ECFMP mandatory route / groundstop CTOT
const COLOR_ECFMP_RED       = "#FF0000"; // ECFMP groundstop destination / prohibit FL

const ecfmpStyle = (type: "yellow" | "red") => type === "yellow"
  ? { backgroundColor: COLOR_ECFMP_YELLOW }
  : { backgroundColor: COLOR_ECFMP_RED, color: "white" };

  function useEditableField(value: string | number | undefined | null) {
  const [fieldValue, setFieldValue] = useState(value?.toString() ?? "");
  const [focused, setFocused] = useState(false);

  const handleSetFocused = (nextFocused: boolean) => {
    if (nextFocused) {
      setFieldValue(value?.toString() ?? "");
    }

    setFocused(nextFocused);
  };

  const displayValue = focused ? fieldValue : value?.toString() ?? "";

  return [displayValue, setFieldValue, focused, handleSetFocused] as const;
}

type ClxField = "sid" | "runway" | "rnav" | "eobt" | "tobt";

function hasClxFieldFault(strip: { clx_validation?: { faults: { fields: string[] }[] } } | undefined, field: ClxField) {
  return strip?.clx_validation?.faults.some(fault => fault.fields.includes(field)) ?? false;
}

function clxFieldStyle(hasFault: boolean) {
  return hasFault ? { backgroundColor: COLOR_CLX_ERROR } : {};
}

function invalidPhaseTobtStyle(phase?: string) {
  return phase === "I" ? { backgroundColor: CDM_RED, color: "white" } : {};
}

function firstMandatoryRouteToken(route: string) {
  for (const token of route
    .toUpperCase()
    .split(/[^A-Z0-9/]+/)
    .map((value) => value.trim())
    .filter(Boolean)) {
    if (token !== "DCT") {
      return token;
    }
  }

  return "";
}

function sidFamily(sid?: string) {
  const value = sid?.trim().toUpperCase() ?? "";
  if (!value) return "";

  const match = value.match(/^[A-Z]+/);
  return match?.[0] ?? value;
}

function sidVariant(sid?: string) {
  const value = sid?.trim().toUpperCase() ?? "";
  const family = sidFamily(value);
  return family.length >= value.length ? "" : value.slice(family.length);
}

function resolveMandatoryRouteSid(route: string, runway: string | undefined, currentSid: string | undefined, availableSids: SidInfo[]) {
  const family = sidFamily(firstMandatoryRouteToken(route));
  if (!family) return "";

  const candidates = availableSids
    .filter((sid) => sidFamily(sid.name) === family && (!runway || sid.runway === runway))
    .map((sid) => sid.name.trim().toUpperCase())
    .filter(Boolean)
    .sort();

  if (candidates.length === 0) {
    return "";
  }

  const currentVariant = sidVariant(currentSid);
  if (currentVariant) {
    const matchingVariant = candidates.find((candidate) => sidVariant(candidate) === currentVariant);
    if (matchingVariant) {
      return matchingVariant;
    }
  }

  const normalizedCurrentSid = currentSid?.trim().toUpperCase() ?? "";
  if (normalizedCurrentSid) {
    const exactMatch = candidates.find((candidate) => candidate === normalizedCurrentSid);
    if (exactMatch) {
      return exactMatch;
    }
  }

  return candidates[0];
}

function selectMandatoryRoute(currentRoute: string | undefined, routes: string[]) {
  const normalizedCurrentRoute = currentRoute?.trim().toUpperCase() ?? "";
  const normalizedRoutes = routes
    .map((route) => route.trim().toUpperCase())
    .filter(Boolean);

  if (normalizedCurrentRoute) {
    const matchingRoute = normalizedRoutes.find((route) => route === normalizedCurrentRoute);
    if (matchingRoute) {
      return matchingRoute;
    }
  }

  return normalizedRoutes[0] ?? "";
}

function clxNitosRemarks(strip: { clx_validation?: { faults: { nitos_remark: string }[] }, pdc_request_remarks?: string, ecfmp_restrictions?: import("@/api/models").EcfmpRestriction[] } | undefined) {
  const ecfmpRemarks = getEcfmpNitosRemarks(strip?.ecfmp_restrictions);
  const clxRemarks = strip?.clx_validation?.faults
    .map(fault => fault.nitos_remark.trim())
    .filter(Boolean) ?? [];
  const pdcRemarks = strip?.pdc_request_remarks?.trim();
  const remarks = [...ecfmpRemarks, ...clxRemarks];
  if (pdcRemarks) remarks.push(pdcRemarks);
  return Array.from(new Set(remarks)).join(" ");
}

function fittedNitosRemarksStyle(value: string) {
  const compactLength = value.replace(/\s+/g, " ").trim().length;
  const multiline = compactLength > NITOS_MULTILINE_LENGTH;
  const fitLength = multiline ? NITOS_MULTILINE_LENGTH * 2 : NITOS_SINGLE_LINE_FIT_LENGTH;
  const fontSize = compactLength > fitLength
    ? Math.max(MIN_FITTED_FIELD_FONT_SIZE, Math.floor((20 * fitLength * 10) / compactLength) / 10)
    : 20;

  return {
    fontSize: scalePx(fontSize),
    lineHeight: multiline ? 1.15 : FIELD_HEIGHT,
    whiteSpace: multiline ? "normal" as const : "pre" as const,
    overflowWrap: multiline ? "break-word" as const : undefined,
    paddingBlock: multiline ? scalePx(3) : 0,
    rows: multiline ? 3 : 1,
    wrap: multiline ? "soft" : "off",
  };
}

function fittedSpokenCallsignStyle(value: string) {
  const compactLength = value.replace(/\s+/g, " ").trim().length;
  const fontSize = compactLength > SPOKEN_CS_FIT_LENGTH
    ? Math.max(MIN_FITTED_FIELD_FONT_SIZE, Math.floor((20 * SPOKEN_CS_FIT_LENGTH * 10) / compactLength) / 10)
    : 20;
  return { fontSize: scalePx(fontSize) };
}

function sidOverrideKey(strip: { clx_validation?: { faults: { fields: string[], override_key?: string }[] } } | undefined) {
  return strip?.clx_validation?.faults.find(fault => fault.override_key && fault.fields.includes("sid"))?.override_key;
}

interface FlightPlanDialogProps {
  callsign: string;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  children?: React.ReactNode;
  mode?: "clearance" | "view";
  pdcAction?: "default" | "manual";
}

export default function FlightPlanDialog({
  callsign,
  open,
  onOpenChange,
  children,
  mode = "clearance",
  pdcAction = "default",
}: FlightPlanDialogProps) {
  const isViewMode = mode === "view";
  const usesManualPdcBypass = pdcAction === "manual";
  const strip = useStrip(callsign);
  const initialCflByRunway = useInitialCflByRunway();
  const transitionAltitude = useTransitionAltitude();
  const moveAction = useWebSocketStore((state) => state.move);
  const generateSquawk = useWebSocketStore((state) => state.generateSquawk);
  const clearPdc = useWebSocketStore((state) => state.issuePdcClearance);
  const revertToVoice = useWebSocketStore((state) => state.revertToVoice);
  const updateStrip = useWebSocketStore((state) => state.updateStrip);
  const clxUpdateTobt = useWebSocketStore((state) => state.clxUpdateTobt);
  const clxOverrideValidation = useWebSocketStore((state) => state.clxOverrideValidation);
  const sendPrivateMessage = useWebSocketStore((state) => state.sendPrivateMessage);

  const [internalOpen, setInternalOpen] = useState(false);
  const dialogOpen = open ?? internalOpen;
  const setDialogOpen = onOpenChange ?? setInternalOpen;

  const displayedEobt = normalizeCdmTime(strip?.eobt);
  const displayedTobt = normalizeCdmTime(strip?.tobt);
  const displayedTsat = normalizeCdmTime(strip?.tsat);
  const mandatoryRoute = getMandatoryRouteRestriction(strip?.ecfmp_restrictions);
  const mandatoryRouteToClear = selectMandatoryRoute(strip?.route, mandatoryRoute?.routes ?? []);
  const hasMandatoryRoute = !!mandatoryRoute;
  const hasGroundStop = !!getGroundStopRestriction(strip?.ecfmp_restrictions);
  const hasProhibit = !!getProhibitRestriction(strip?.ecfmp_restrictions);
  const flViolated = isFlightLevelViolated(strip?.ecfmp_restrictions, strip?.requested_altitude);
  const ctotSlotDisplay = getCtotSlotDisplay({
    ctot: strip?.ctot,
    most_penalizing_airspace: strip?.most_penalizing_airspace,
  });

  const [sidDialogOpen, setSidDialogOpen] = useState(false);
  const [rnavDialogOpen, setRnavDialogOpen] = useState(false);
  const [rwyDialogOpen, setRwyDialogOpen] = useState(false);
  const availableSids = useAvailableSids();
  const mandatoryRouteSid = strip
    ? resolveMandatoryRouteSid(mandatoryRouteToClear, strip.runway, strip.sid, availableSids)
    : "";
  const mandatoryRouteSidMismatch = !!mandatoryRouteSid && mandatoryRouteSid !== (strip?.sid?.trim().toUpperCase() ?? "");
  const [ssrGenerating, setSsrGenerating] = useState(false);
  const [standOpen, setStandOpen] = useState(false);
  const [eobt, setEobt, _eobtFocused, setEobtFocused] = useEditableField(displayedEobt);

  // Clear SSR loading state when the backend updates assigned_squawk
  const prevSquawkRef = useRef(strip?.assigned_squawk);
  if (prevSquawkRef.current !== strip?.assigned_squawk) {
    prevSquawkRef.current = strip?.assigned_squawk;
    if (ssrGenerating) setSsrGenerating(false);
  }
  const [route, setRoute, _routeFocused, setRouteFocused] = useEditableField(strip?.route);
  const [hdgDialogOpen, setHdgDialogOpen] = useState(false);
  const [altDialogOpen, setAltDialogOpen] = useState(false);
  const [mandatoryRouteDialogOpen, setMandatoryRouteDialogOpen] = useState(false);
  const [manualPdcBypassDialogOpen, setManualPdcBypassDialogOpen] = useState(false);
  const defaultClearedAltitude = strip?.runway ? initialCflByRunway[strip.runway] : undefined;
  const commitEobt = () => updateStrip(callsign, { eobt: normalizeCdmTime(eobt) });
  const sidFault = hasClxFieldFault(strip, "sid");
  const runwayFault = hasClxFieldFault(strip, "runway");
  const rnavFault = hasClxFieldFault(strip, "rnav");
  const eobtFault = hasClxFieldFault(strip, "eobt");
  const tobtFault = hasClxFieldFault(strip, "tobt");
  const sidOverride = sidOverrideKey(strip);
  const nitosRemarks = clxNitosRemarks(strip);
  const nitosRemarksFit = fittedNitosRemarksStyle(nitosRemarks);
  const spokenCallsign = strip.spoken_callsign ?? "";
  const spokenCallsignFit = fittedSpokenCallsignStyle(spokenCallsign);
  const isPdcRequest = strip?.pdc_state === "REQUESTED" || strip?.pdc_state === "REQUESTED_WITH_FAULTS";
  const fieldStyle = (width: number) => ({
    width: scalePx(width),
    height: FIELD_HEIGHT,
    fontFamily: FONT_FAMILY,
    fontSize: FONT_SIZE_FIELD,
  });
  const gridGroupStyle = { gap: FIELD_GAP };
  const rowStyle = { width: CONTENT_WIDTH, gap: FIELD_GAP };
  const actionButtonStyle = {
    width: ACTION_BUTTON_WIDTH,
    height: ACTION_BUTTON_HEIGHT,
    fontFamily: FONT_FAMILY,
    fontWeight: "bold" as const,
    fontSize: FONT_SIZE_BUTTON,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    cursor: "pointer",
  };

  const handleRnavChange = (capability: RnavCapability) => {
    if (!strip) return;
    const update = buildRnavUpdate(strip.aircraft_type ?? "", strip.remarks ?? "", capability);
    updateStrip(callsign, update);
  };

  const performManualClearance = () => {
    if (!strip) return;

    moveAction(strip.callsign, Bay.Cleared);
    setManualPdcBypassDialogOpen(false);
    setDialogOpen(false);
  };

  return (
    <>
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        {children ? <DialogTrigger asChild>{children}</DialogTrigger> : null}
        {strip ? (
        <DialogContent
          className={CLS_DIALOG}
          style={{ width: DIALOG_WIDTH, maxWidth: DIALOG_WIDTH, height: isViewMode ? VIEW_DIALOG_HEIGHT : CLEARANCE_DIALOG_HEIGHT, maxHeight: isViewMode ? VIEW_DIALOG_HEIGHT : CLEARANCE_DIALOG_HEIGHT, padding: DIALOG_PADDING }}
        >
          <VisuallyHidden.Root>
            <DialogTitle>Flight plan</DialogTitle>
          </VisuallyHidden.Root>
          <div
            className={`relative border-2 border-black flex flex-col items-center ${isViewMode ? "" : "flex-1 min-h-0"}`}
            style={{ gap: PANEL_PADDING, paddingTop: PANEL_PADDING, paddingBottom: PANEL_PADDING, color: "black" }}
          >
          <span
            className={CLS_DIALOG_LABEL}
            style={{ top: `calc(-1 * ${LABEL_OFFSET})`, left: "50%", transform: "translateX(-50%)", whiteSpace: "nowrap", paddingInline: scalePx(5) }}
          >
            {isViewMode ? "DEPARTURE" : "FLIGHT PLAN"}
          </span>

          <div className="flex" style={rowStyle}>
            <div className="grid items-center" style={gridGroupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>C/S</Label>
              <Input
                value={strip.callsign}
                disabled
                className={CLS_BTN_DISABLED}
                style={fieldStyle(180)}
              />
            </div>
            <div className="grid items-center" style={gridGroupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>ADES</Label>
              <Input
                value={strip.destination}
                disabled
                className={CLS_BTN_DISABLED}
                style={{ ...fieldStyle(100), ...(hasGroundStop ? ecfmpStyle("red") : {}) }}
              />
            </div>
            <div className="grid items-center" style={gridGroupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>RNAV</Label>
              <button
                type="button"
                onClick={() => setRnavDialogOpen(true)}
                className={CLS_BTN_EDITABLE}
                style={{ ...fieldStyle(75), ...clxFieldStyle(rnavFault) }}
              >
                {strip.capabilities ?? "NIL"}
              </button>
              <RnavSelectDialog
                open={rnavDialogOpen}
                onOpenChange={setRnavDialogOpen}
                value={strip.capabilities}
                onSelect={handleRnavChange}
              />
            </div>
            <div className="grid items-center" style={gridGroupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>SID</Label>
              <button
                type="button"
                onClick={() => setSidDialogOpen(true)}
                className={CLS_BTN_EDITABLE}
                style={{ ...fieldStyle(150), ...clxFieldStyle(sidFault) }}
              >
                {strip.sid ?? ""}
              </button>
              <SidSelectDialog
                open={sidDialogOpen}
                onOpenChange={setSidDialogOpen}
                value={strip.sid}
                onSelect={(sid) => {
                  if (sid === strip.sid && sidOverride) {
                    clxOverrideValidation(callsign, sidOverride);
                    return;
                  }
                  updateStrip(callsign, { sid });
                }}
                sids={availableSids.length > 0 ? availableSids.filter(s => s.runway === strip.runway).map(s => s.name) : undefined}
              />
            </div>
            <div className="grid items-center" style={gridGroupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>SSR</Label>
              <Button
                className={CLS_BTN_EDITABLE_LOCK}
                style={{
                  ...fieldStyle(100),
                  opacity: ssrGenerating ? 0.5 : 1,
                }}
                disabled={ssrGenerating}
                onClick={() => {
                  if (generateSquawk(callsign)) {
                    setSsrGenerating(true);
                  }
                }}
              >
                {strip.assigned_squawk}
              </Button>
            </div>
            <div className="grid items-center" style={gridGroupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>CTOT</Label>
              <div className="flex">
                <input
                  value={ctotSlotDisplay.restrictionLabel}
                  title={ctotSlotDisplay.restrictionLabel}
                  disabled
                  className={CLS_BTN_DISABLED_LEFT}
                  style={fieldStyle(100)}
                />
                <input
                  value={ctotSlotDisplay.ctot}
                  disabled
                  className={CLS_BTN_DISABLED}
                  style={{ ...fieldStyle(100), ...(ctotSlotDisplay.hasCtot ? ecfmpStyle("yellow") : {}) }}
                />
              </div>
            </div>
          </div>

          <div className="flex justify-between" style={rowStyle}>
            <div className="flex" style={{ gap: FIELD_GAP }}>
              <div className="grid items-center" style={gridGroupStyle}>
                <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>EOBT</Label>
                <input
                  value={eobt}
                  onChange={(event) => setEobt(event.target.value)}
                  onFocus={() => setEobtFocused(true)}
                  onBlur={() => {
                    setEobtFocused(false);
                    commitEobt();
                  }}
                  onKeyDown={(event) => event.key === "Enter" && commitEobt()}
                  className={CLS_BTN_EDITABLE}
                  style={{ ...fieldStyle(100), ...clxFieldStyle(eobtFault) }}
                />
              </div>
              <div className="grid items-center" style={gridGroupStyle}>
                <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>TOBT</Label>
                <button
                  type="button"
                  onClick={() => {
                    if (tobtFault) clxUpdateTobt(callsign);
                  }}
                  className={CLS_BTN_DISABLED}
                  style={{ ...fieldStyle(100), ...clxFieldStyle(tobtFault), ...invalidPhaseTobtStyle(strip.phase), cursor: tobtFault ? "pointer" : undefined }}
                >
                  {displayedTobt}
                </button>
              </div>
              <div className="grid items-center" style={gridGroupStyle}>
                <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>TSAT</Label>
                <Input
                  value={displayedTsat}
                  disabled
                  className={CLS_BTN_DISABLED}
                  style={fieldStyle(100)}
                />
              </div>
              <div className="grid items-center" style={gridGroupStyle}>
                <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>RWY</Label>
                <button
                  type="button"
                  onClick={() => setRwyDialogOpen(true)}
                  className={CLS_BTN_EDITABLE}
                  style={{ ...fieldStyle(150), ...clxFieldStyle(runwayFault) }}
                >
                  {strip.runway}
                </button>
                <RunwayDialog
                  mode="ASSIGN"
                  open={rwyDialogOpen}
                  onOpenChange={setRwyDialogOpen}
                  callsign={callsign}
                  direction="departure"
                  currentRunway={strip.runway}
                />
              </div>
            </div>
            <div className="grid items-center" style={gridGroupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>REA</Label>
              <Input
                value={strip.release_point}
                disabled
                className={CLS_BTN_DISABLED}
                style={fieldStyle(100)}
              />
            </div>
          </div>

          <div className="flex" style={rowStyle}>
            <div className="grid items-center" style={gridGroupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>TYPE</Label>
              <Input
                value={getAircraftTypeWithWtc(strip.aircraft_type, strip.aircraft_category)}
                disabled
                className={CLS_BTN_DISABLED_LEFT}
                style={fieldStyle(200)}
              />
            </div>
            <div className="grid items-center" style={gridGroupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>FL</Label>
              <Input
                value={strip.requested_altitude ? Math.floor(strip.requested_altitude / 100).toString() : ""}
                disabled
                className={CLS_BTN_DISABLED_LEFT}
                style={{ ...fieldStyle(100), ...(hasProhibit && flViolated ? ecfmpStyle("red") : {}) }}
              />
            </div>
            <div className="grid items-center" style={gridGroupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>SPEED</Label>
              <Input
                defaultValue=""
                disabled
                className={CLS_BTN_DISABLED_LEFT}
                style={fieldStyle(100)}
              />
            </div>
            <div className="grid items-center" style={gridGroupStyle}>
              <Label className="font-light text-center" style={{ fontSize: FONT_SIZE_LABEL }}>STS</Label>
              <Input
                defaultValue=""
                disabled
                className={CLS_BTN_DISABLED}
                style={fieldStyle(420)}
              />
            </div>
          </div>

          <div className="flex flex-col" style={{ width: CONTENT_WIDTH, gap: FIELD_GAP }}>
            <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>ROUTE</Label>
            <textarea
              value={hasMandatoryRoute && mandatoryRouteToClear ? mandatoryRouteToClear : route}
              onChange={(event) => setRoute(event.target.value)}
              onFocus={() => setRouteFocused(true)}
              onBlur={() => {
                setRouteFocused(false);
                updateStrip(callsign, { route });
              }}
              onKeyDown={(event) => event.key === "Enter" && !event.shiftKey && updateStrip(callsign, { route })}
              className={CLS_TEXTAREA_EDITABLE}
              disabled={hasMandatoryRoute}
              style={{ height: TEXTAREA_HEIGHT, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD, ...(hasMandatoryRoute ? { backgroundColor: COLOR_ECFMP_YELLOW } : {}) }}
            />
          </div>

          <div className="flex flex-col" style={{ width: CONTENT_WIDTH, gap: FIELD_GAP }}>
            <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>COOPANS REMARKS</Label>
            <Input
              value={strip.remarks}
              disabled
              className={`${CLS_BTN_DISABLED_NRM} w-full`}
              style={{ height: FIELD_HEIGHT, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
            />
          </div>

          <div className="flex" style={rowStyle}>
            <div className="grid items-center" style={gridGroupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>NITOS REMARKS</Label>
              <textarea
                aria-label="NITOS remarks"
                title={nitosRemarks}
                value={nitosRemarks}
                disabled
                readOnly
                rows={nitosRemarksFit.rows}
                wrap={nitosRemarksFit.wrap}
                className={CLS_NITOS_REMARKS}
                style={{
                  ...fieldStyle(700),
                  fontSize: nitosRemarksFit.fontSize,
                  lineHeight: nitosRemarksFit.lineHeight,
                  whiteSpace: nitosRemarksFit.whiteSpace,
                  overflowWrap: nitosRemarksFit.overflowWrap,
                  paddingInline: NITOS_REMARKS_INLINE_PADDING,
                  paddingBlock: nitosRemarksFit.paddingBlock,
                }}
              />
            </div>
            <div className="grid items-center" style={gridGroupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>SPOKEN C/S</Label>
              <Input
                value={spokenCallsign}
                disabled
                className={CLS_BTN_DISABLED}
                style={{
                  ...fieldStyle(130),
                  fontSize: spokenCallsignFit.fontSize,
                }}
              />
            </div>
          </div>

          <div className="flex justify-between" style={{ width: CONTENT_WIDTH }}>
            <div className="grid items-center" style={gridGroupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>CLIMB GR.</Label>
              <Input
                defaultValue=""
                disabled
                className={CLS_BTN_DISABLED}
                style={fieldStyle(125)}
              />
            </div>
            <div className="grid items-center" style={gridGroupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>HDG</Label>
              <button
                type="button"
                onClick={() => setHdgDialogOpen(true)}
                className={CLS_BTN_EDITABLE}
                style={fieldStyle(125)}
              >
                {strip.heading ? strip.heading.toString().padStart(3, "0") : ""}
              </button>
              <HdgSelectDialog
                open={hdgDialogOpen}
                onOpenChange={setHdgDialogOpen}
                value={strip.heading}
                onSelect={(heading) => updateStrip(callsign, { heading: heading ?? 0 })}
              />
            </div>
            <div className="grid items-center" style={gridGroupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>ALT</Label>
              <button
                type="button"
                onClick={() => setAltDialogOpen(true)}
                className={CLS_BTN_EDITABLE}
                style={fieldStyle(125)}
              >
                {strip.cleared_altitude ? formatAltitude(strip.cleared_altitude, transitionAltitude) : ""}
              </button>
              <AltSelectDialog
                open={altDialogOpen}
                onOpenChange={setAltDialogOpen}
                value={strip.cleared_altitude}
                onSelect={(altitude) => {
                  updateStrip(callsign, {
                    altitude: altitude ?? defaultClearedAltitude ?? strip.cleared_altitude,
                  });
                }}
              />
            </div>
            <div className="grid items-center" style={gridGroupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>DE-ICE</Label>
              <Input
                defaultValue=""
                disabled
                className={CLS_BTN_DISABLED}
                style={fieldStyle(125)}
              />
            </div>
            <div className="grid items-center" style={gridGroupStyle}>
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>REG</Label>
              <Input
                value={strip.registration ?? ""}
                disabled
                className={CLS_BTN_DISABLED}
                style={fieldStyle(125)}
              />
            </div>
            <div className="grid items-center" style={{ width: scalePx(125), gap: FIELD_GAP }}>
              <Label htmlFor="stand" className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>
                Stand
              </Label>
              <Input
                id="stand"
                value={strip.stand ?? ""}
                readOnly
                onClick={() => {
                  setDialogOpen(false);
                  setStandOpen(true);
                }}
                className="border-black rounded-none text-black font-bold w-full text-center cursor-pointer"
                style={{ height: FIELD_HEIGHT, fontFamily: FONT_FAMILY, fontSize: scalePx(18) }}
              />
            </div>
          </div>
        </div>

          {isViewMode && (
            <div
              className="relative border-2 border-black flex flex-col items-center border-t-0"
              style={{ paddingTop: COMPACT_PANEL_PADDING, paddingBottom: COMPACT_PANEL_PADDING, color: "black" }}
            >
              <span
                className={CLS_DIALOG_LABEL}
                style={{ top: `calc(-1 * ${LABEL_OFFSET})`, left: "50%", transform: "translateX(-50%)", whiteSpace: "nowrap", paddingInline: scalePx(5) }}
              >
                ARRIVAL
              </span>
              <div className="flex justify-between" style={{ width: CONTENT_WIDTH }}>
                <div className="grid items-center" style={gridGroupStyle}>
                  <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>ADEP</Label>
                  <Input
                    value={strip.origin}
                    disabled
                    className={CLS_BTN_DISABLED}
                    style={fieldStyle(150)}
                  />
                </div>
                <div className="grid items-center" style={gridGroupStyle}>
                  <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>STAR</Label>
                  <Input
                    defaultValue=""
                    disabled
                    className={CLS_BTN_DISABLED}
                    style={fieldStyle(150)}
                  />
                </div>
                <div className="grid items-center" style={gridGroupStyle}>
                  <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>RWY</Label>
                  <Input
                    defaultValue=""
                    disabled
                    className={CLS_BTN_DISABLED}
                    style={fieldStyle(150)}
                  />
                </div>
                <div className="grid items-center" style={gridGroupStyle}>
                  <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>ETA</Label>
                  <Input
                    value={strip.eldt ?? ""}
                    disabled
                    className={CLS_BTN_DISABLED}
                    style={fieldStyle(150)}
                  />
                </div>
                <div className="grid items-center" style={gridGroupStyle}>
                  <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>AOBT</Label>
                  <Input
                    defaultValue=""
                    disabled
                    className={CLS_BTN_DISABLED}
                    style={fieldStyle(150)}
                  />
                </div>
              </div>
            </div>
          )}

          <div className="flex flex-row items-center justify-center" style={{ paddingTop: isViewMode ? scalePx(8) : scalePx(12) }}>
            {isViewMode ? (
              <button
                onClick={() => setDialogOpen(false)}
                style={{
                  ...actionButtonStyle,
                  backgroundColor: COLOR_DARK_BTN,
                  color: "white",
                }}
              >
                OK
              </button>
            ) : (
              <>
                <button
                  onClick={() => setDialogOpen(false)}
                  style={{
                    ...actionButtonStyle,
                    backgroundColor: COLOR_DARK_BTN,
                    color: "white",
                  }}
                >
                  ESC
                </button>
                <div className="ml-auto flex flex-row items-center" style={{ gap: scalePx(8) }}>
                  {(strip.pdc_state === "REQUESTED" || strip.pdc_state === "REQUESTED_WITH_FAULTS") && (
                    <button
                      onClick={() => {
                        revertToVoice(strip.callsign);
                        setDialogOpen(false);
                      }}
                      style={{
                        height: ACTION_BUTTON_HEIGHT,
                        fontFamily: FONT_FAMILY,
                        fontWeight: "bold",
                        fontSize: FONT_SIZE_BUTTON,
                        backgroundColor: COLOR_REVERT_BTN,
                        color: "black",
                        padding: `${scalePx(4)} ${scalePx(12)}`,
                        whiteSpace: "nowrap",
                      }}
                    >
                      REVERT TO VOICE
                    </button>
                  )}
                  <button
                    onClick={() => {
                      if (hasMandatoryRoute) {
                        setMandatoryRouteDialogOpen(true);
                        return;
                      }
                      if ((strip.pdc_state === "REQUESTED" || strip.pdc_state === "REQUESTED_WITH_FAULTS") && usesManualPdcBypass) {
                        setManualPdcBypassDialogOpen(true);
                        return;
                      }
                      if (strip.pdc_state === "REQUESTED" || strip.pdc_state === "REQUESTED_WITH_FAULTS") {
                        clearPdc(strip.callsign, null);
                      } else {
                        moveAction(strip.callsign, Bay.Cleared);
                      }

                      setDialogOpen(false);
                    }}
                    style={{
                      ...actionButtonStyle,
                      backgroundColor: "#3F3F3F",
                      color: "#FFFFFF",
                    }}
                  >
                    CLD
                  </button>
                </div>
              </>
            )}
          </div>
        </DialogContent>
        ) : (
        <DialogContent className={CLS_CLX_DIALOG} style={{ width: CLX_FALLBACK_WIDTH, padding: scalePx(16) }}>
          <VisuallyHidden.Root>
            <DialogTitle>Flight plan unavailable</DialogTitle>
          </VisuallyHidden.Root>
          <div className={CLS_CLX_PANEL} style={{ padding: scalePx(16) }}>
            <div className="bg-white text-center font-semibold" style={{ padding: `${scalePx(12)} ${scalePx(16)}`, fontSize: scalePx(20) }}>
              Flight plan unavailable
            </div>
            <p className="text-center" style={{ marginTop: scalePx(16), fontSize: scalePx(14) }}>
              The selected strip is no longer visible in the active bays.
            </p>
            <div style={{ marginTop: scalePx(16) }}>
              <Button variant="darkaction" className="w-full" style={{ height: CLX_FALLBACK_BUTTON_HEIGHT, fontSize: FONT_SIZE_BUTTON }} onClick={() => setDialogOpen(false)}>
                ESC
              </Button>
            </div>
          </div>
        </DialogContent>
        )}
      </Dialog>

      {strip && (
        <ArrStandDialog
          open={standOpen}
          onOpenChange={setStandOpen}
          callsign={callsign}
          currentStand={strip.stand}
        />
      )}

      {strip && (
        <MandatoryRouteDialog
          open={mandatoryRouteDialogOpen}
          onOpenChange={setMandatoryRouteDialogOpen}
          callsign={callsign}
          route={mandatoryRouteToClear}
          filedSid={strip.sid}
          mandatorySid={mandatoryRouteSid}
          sidMismatch={mandatoryRouteSidMismatch}
          pdcRequested={isPdcRequest}
          onConfirm={() => {
            if (isPdcRequest) {
              clearPdc(strip.callsign, null);
            } else {
              if (mandatoryRouteSidMismatch) {
                updateStrip(callsign, { sid: mandatoryRouteSid });
              }
              if (mandatoryRouteToClear && mandatoryRouteToClear !== strip.route?.trim().toUpperCase()) {
                updateStrip(callsign, { route: mandatoryRouteToClear });
              }
              if (mandatoryRouteToClear) {
                sendPrivateMessage(callsign, `MANDATORY ROUTE: ${mandatoryRouteToClear}`);
              }
              moveAction(strip.callsign, Bay.Cleared);
            }
            setMandatoryRouteDialogOpen(false);
            setDialogOpen(false);
          }}
          onCancel={() => {
            moveAction(strip.callsign, Bay.Cleared);
            setMandatoryRouteDialogOpen(false);
            setDialogOpen(false);
          }}
        />
      )}

      {strip && (
        <ManualPdcBypassDialog
          open={manualPdcBypassDialogOpen}
          onOpenChange={setManualPdcBypassDialogOpen}
          callsign={callsign}
          onConfirm={performManualClearance}
        />
      )}
    </>
  );
}
