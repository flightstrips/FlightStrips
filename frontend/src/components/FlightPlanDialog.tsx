import React, { useRef, useState } from "react";
import * as VisuallyHidden from "@radix-ui/react-visually-hidden";

import { Bay } from "@/api/models.ts";
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
import { useAvailableSids, useInitialCflByRunway, useStrip, useTransitionAltitude, useWebSocketStore } from "@/store/store-hooks.ts";

const FONT_FAMILY = "Arial";
const FONT_SIZE_FIELD = 20;
const FONT_SIZE_LABEL = 16;
const FONT_SIZE_BUTTON = 24;

// Tailwind class constants (hex must be literal strings for JIT)
const CLS_DIALOG         = "bg-[#d4d4d4] rounded-none p-[25px] flex flex-col gap-0";
const CLS_DIALOG_LABEL   = "absolute bg-[#d4d4d4] px-[5px] text-black font-bold";
const CLS_BTN_DISABLED      = "border border-black rounded-none bg-[#b3b3b3] text-black font-bold h-[50px] text-center disabled:opacity-60";
const CLS_BTN_DISABLED_BDR  = "border-2 border-black rounded-none bg-[#b3b3b3] text-black font-bold h-[50px] text-center disabled:opacity-60";
const CLS_BTN_DISABLED_LEFT = "border border-r-0 border-black rounded-none bg-[#b3b3b3] text-black font-bold h-[50px] text-center disabled:opacity-60";
const CLS_BTN_DISABLED_NRM  = "border border-black rounded-none bg-[#b3b3b3] text-black font-bold h-[50px] text-center disabled:opacity-60";
const CLS_BTN_EDITABLE      = "border border-black rounded-none bg-[#ededed] text-black font-bold h-[50px] text-center";
const CLS_BTN_EDITABLE_LOCK = "border border-black rounded-none bg-[#ededed] text-black font-bold disabled:opacity-100 h-[50px] text-center select-none hover:bg-[#ededed]";
const CLS_TEXTAREA_EDITABLE = "border border-black rounded-none bg-[#ededed] text-black font-normal text-center h-[80px] break-words resize-none w-full";
const CLS_CLX_DIALOG        = "w-[360px] rounded-none border border-black bg-[#B3B3B3] p-4 text-black";
const CLS_CLX_PANEL         = "border border-black bg-[#D6D6D6] p-4";
// Style-prop color constants (used in CSSProperties, not Tailwind)
const COLOR_DARK_BTN        = "#3F3F3F"; // dark ESC/CLD button
const COLOR_REVERT_BTN      = "#FFFB03"; // yellow revert-to-voice button

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

interface FlightPlanDialogProps {
  callsign: string;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  children?: React.ReactNode;
  mode?: "clearance" | "view";
}

export default function FlightPlanDialog({
  callsign,
  open,
  onOpenChange,
  children,
  mode = "clearance",
}: FlightPlanDialogProps) {
  const isViewMode = mode === "view";
  const strip = useStrip(callsign);
  const initialCflByRunway = useInitialCflByRunway();
  const transitionAltitude = useTransitionAltitude();
  const moveAction = useWebSocketStore((state) => state.move);
  const generateSquawk = useWebSocketStore((state) => state.generateSquawk);
  const clearPdc = useWebSocketStore((state) => state.issuePdcClearance);
  const revertToVoice = useWebSocketStore((state) => state.revertToVoice);
  const updateStrip = useWebSocketStore((state) => state.updateStrip);

  const [internalOpen, setInternalOpen] = useState(false);
  const dialogOpen = open ?? internalOpen;
  const setDialogOpen = onOpenChange ?? setInternalOpen;

  const [sidDialogOpen, setSidDialogOpen] = useState(false);
  const [rwyDialogOpen, setRwyDialogOpen] = useState(false);
  const availableSids = useAvailableSids();
  const [ssrGenerating, setSsrGenerating] = useState(false);
  const [standOpen, setStandOpen] = useState(false);
  const [eobt, setEobt, _eobtFocused, setEobtFocused] = useEditableField(strip?.eobt);

  // Clear SSR loading state when the backend updates assigned_squawk
  const prevSquawkRef = useRef(strip?.assigned_squawk);
  if (prevSquawkRef.current !== strip?.assigned_squawk) {
    prevSquawkRef.current = strip?.assigned_squawk;
    if (ssrGenerating) setSsrGenerating(false);
  }
  const [route, setRoute, _routeFocused, setRouteFocused] = useEditableField(strip?.route);
  const [hdgDialogOpen, setHdgDialogOpen] = useState(false);
  const [altDialogOpen, setAltDialogOpen] = useState(false);
  const defaultClearedAltitude = strip?.runway ? initialCflByRunway[strip.runway] : undefined;

  return (
    <>
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        {children ? <DialogTrigger asChild>{children}</DialogTrigger> : null}
        {strip ? (
        <DialogContent
          className={CLS_DIALOG}
          style={{ width: 1000, maxWidth: 1000, height: isViewMode ? 1015 : 925, maxHeight: isViewMode ? 1015 : 925 }}
        >
          <VisuallyHidden.Root>
            <DialogTitle>Flight plan</DialogTitle>
          </VisuallyHidden.Root>
          <div
            className={`relative border-2 border-black flex flex-col items-center gap-[30px] ${isViewMode ? "" : "flex-1 min-h-0"}`}
            style={{ paddingTop: 30, paddingBottom: 30, color: "black" }}
          >
          <span
            className={CLS_DIALOG_LABEL}
            style={{ top: -11, left: "50%", transform: "translateX(-50%)", whiteSpace: "nowrap" }}
          >
            {isViewMode ? "DEPARTURE" : "FLIGHT PLAN"}
          </span>

          <div className="flex gap-[5px]" style={{ width: 835 }}>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>C/S</Label>
              <Input
                value={strip.callsign}
                disabled
                className={CLS_BTN_DISABLED}
                style={{ width: 180, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>ADES</Label>
              <Input
                value={strip.destination}
                disabled
                className={CLS_BTN_DISABLED}
                style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>RNAV</Label>
              <Input
                value={strip.capabilities}
                disabled
                className={CLS_BTN_DISABLED}
                style={{ width: 75, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>SID</Label>
              <button
                type="button"
                onClick={() => setSidDialogOpen(true)}
                className={CLS_BTN_EDITABLE}
                style={{ width: 150, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              >
                {strip.sid ?? ""}
              </button>
              <SidSelectDialog
                open={sidDialogOpen}
                onOpenChange={setSidDialogOpen}
                value={strip.sid}
                onSelect={(sid) => updateStrip(callsign, { sid })}
                sids={availableSids.length > 0 ? availableSids.filter(s => s.runway === strip.runway).map(s => s.name) : undefined}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>SSR</Label>
              <Button
                className={CLS_BTN_EDITABLE_LOCK}
                style={{
                  width: 100,
                  fontFamily: FONT_FAMILY,
                  fontSize: FONT_SIZE_FIELD,
                  opacity: ssrGenerating ? 0.5 : 1,
                }}
                disabled={ssrGenerating}
                onClick={() => {
                  setSsrGenerating(true);
                  generateSquawk(callsign);
                }}
              >
                {strip.assigned_squawk}
              </Button>
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>TTOT</Label>
              <input
                placeholder=""
                disabled
                className={CLS_BTN_DISABLED_BDR}
                style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>CTOT</Label>
              <input
                value={strip.ctot}
                disabled
                className={CLS_BTN_DISABLED_BDR}
                style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
          </div>

          <div className="flex gap-[5px] justify-between" style={{ width: 835 }}>
            <div className="flex gap-[5px]">
              <div className="grid items-center gap-[5px]">
                <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>EOBT</Label>
                <input
                  value={eobt}
                  onChange={(event) => setEobt(event.target.value)}
                  onFocus={() => setEobtFocused(true)}
                  onBlur={() => {
                    setEobtFocused(false);
                    updateStrip(callsign, { eobt });
                  }}
                  onKeyDown={(event) => event.key === "Enter" && updateStrip(callsign, { eobt })}
                  className={CLS_BTN_EDITABLE}
                  style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
                />
              </div>
              <div className="grid items-center gap-[5px]">
                <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>TOBT</Label>
                <Input
                  value={strip.tobt}
                  disabled
                  className={CLS_BTN_DISABLED}
                  style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
                />
              </div>
              <div className="grid items-center gap-[5px]">
                <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>TSAT</Label>
                <Input
                  value={strip.tsat}
                  disabled
                  className={CLS_BTN_DISABLED}
                  style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
                />
              </div>
              <div className="grid items-center gap-[5px]">
                <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>RWY</Label>
                <button
                  type="button"
                  onClick={() => setRwyDialogOpen(true)}
                  className={CLS_BTN_EDITABLE}
                  style={{ width: 150, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
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
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>REA</Label>
              <Input
                value={strip.release_point}
                disabled
                className={CLS_BTN_DISABLED}
                style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
          </div>

          <div className="flex gap-[5px]" style={{ width: 835 }}>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>TYPE</Label>
              <Input
                value={getAircraftTypeWithWtc(strip.aircraft_type, strip.aircraft_category)}
                disabled
                className={CLS_BTN_DISABLED_LEFT}
                style={{ width: 200, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>FL</Label>
              <Input
                value={strip.requested_altitude ? Math.floor(strip.requested_altitude / 100).toString() : ""}
                disabled
                className={CLS_BTN_DISABLED_LEFT}
                style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>SPEED</Label>
              <Input
                defaultValue=""
                disabled
                className={CLS_BTN_DISABLED_LEFT}
                style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light text-center" style={{ fontSize: FONT_SIZE_LABEL }}>STS</Label>
              <Input
                defaultValue=""
                disabled
                className={CLS_BTN_DISABLED}
                style={{ width: 420, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
          </div>

          <div className="flex flex-col gap-[5px]" style={{ width: 835 }}>
            <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>ROUTE</Label>
            <textarea
              value={route}
              onChange={(event) => setRoute(event.target.value)}
              onFocus={() => setRouteFocused(true)}
              onBlur={() => {
                setRouteFocused(false);
                updateStrip(callsign, { route });
              }}
              onKeyDown={(event) => event.key === "Enter" && !event.shiftKey && updateStrip(callsign, { route })}
              className={CLS_TEXTAREA_EDITABLE}
              style={{ fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
            />
          </div>

          <div className="flex flex-col gap-[5px]" style={{ width: 835 }}>
            <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>COOPANS REMARKS</Label>
            <Input
              value={strip.remarks}
              disabled
              className={`${CLS_BTN_DISABLED_NRM} w-full`}
              style={{ fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
            />
          </div>

          <div className="flex gap-[5px]" style={{ width: 835 }}>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>NITOS REMARKS</Label>
              <Input
                value={strip.pdc_request_remarks ?? ""}
                disabled
                className={CLS_BTN_DISABLED_NRM}
                style={{ width: 700, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>IATA TYPE</Label>
              <Input
                defaultValue=""
                disabled
                className={CLS_BTN_DISABLED}
                style={{ width: 130, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
          </div>

          <div className="flex justify-between" style={{ width: 835 }}>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>CLIMB GR.</Label>
              <Input
                defaultValue=""
                disabled
                className={CLS_BTN_DISABLED}
                style={{ width: 125, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>HDG</Label>
              <button
                type="button"
                onClick={() => setHdgDialogOpen(true)}
                className={CLS_BTN_EDITABLE}
                style={{ width: 125, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
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
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>ALT</Label>
              <button
                type="button"
                onClick={() => setAltDialogOpen(true)}
                className={CLS_BTN_EDITABLE}
                style={{ width: 125, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
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
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>DE-ICE</Label>
              <Input
                defaultValue=""
                disabled
                className={CLS_BTN_DISABLED}
                style={{ width: 125, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>REG</Label>
              <Input
                defaultValue=""
                disabled
                className={CLS_BTN_DISABLED}
                style={{ width: 125, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]" style={{ width: 125 }}>
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
                className="border-black rounded-none text-black font-bold text-[18px] w-full text-center cursor-pointer h-[50px]"
                style={{ fontFamily: FONT_FAMILY }}
              />
            </div>
          </div>
        </div>

          {isViewMode && (
            <div
              className="relative border-2 border-black flex flex-col items-center border-t-0"
              style={{ paddingTop: 20, paddingBottom: 20, color: "black" }}
            >
              <span
                className={CLS_DIALOG_LABEL}
                style={{ top: -11, left: "50%", transform: "translateX(-50%)", whiteSpace: "nowrap" }}
              >
                ARRIVAL
              </span>
              <div className="flex justify-between" style={{ width: 835 }}>
                <div className="grid items-center gap-[5px]">
                  <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>ADEP</Label>
                  <Input
                    value={strip.origin}
                    disabled
                    className={CLS_BTN_DISABLED}
                    style={{ width: 150, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
                  />
                </div>
                <div className="grid items-center gap-[5px]">
                  <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>STAR</Label>
                  <Input
                    defaultValue=""
                    disabled
                    className={CLS_BTN_DISABLED}
                    style={{ width: 150, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
                  />
                </div>
                <div className="grid items-center gap-[5px]">
                  <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>RWY</Label>
                  <Input
                    defaultValue=""
                    disabled
                    className={CLS_BTN_DISABLED}
                    style={{ width: 150, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
                  />
                </div>
                <div className="grid items-center gap-[5px]">
                  <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>ETA</Label>
                  <Input
                    value={strip.eldt ?? ""}
                    disabled
                    className={CLS_BTN_DISABLED}
                    style={{ width: 150, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
                  />
                </div>
                <div className="grid items-center gap-[5px]">
                  <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>AOBT</Label>
                  <Input
                    defaultValue=""
                    disabled
                    className={CLS_BTN_DISABLED}
                    style={{ width: 150, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
                  />
                </div>
              </div>
            </div>
          )}

          <div className={`flex flex-row items-center justify-center ${isViewMode ? "pt-2" : "pt-3"}`}>
            {isViewMode ? (
              <button
                onClick={() => setDialogOpen(false)}
                style={{
                  width: 125,
                  height: 70,
                  backgroundColor: COLOR_DARK_BTN,
                  color: "white",
                  fontFamily: FONT_FAMILY,
                  fontWeight: "bold",
                  fontSize: FONT_SIZE_BUTTON,
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  cursor: "pointer",
                }}
              >
                OK
              </button>
            ) : (
              <>
                <button
                  onClick={() => setDialogOpen(false)}
                  style={{
                    width: 125,
                    height: 70,
                    backgroundColor: COLOR_DARK_BTN,
                    color: "white",
                    fontFamily: FONT_FAMILY,
                    fontWeight: "bold",
                    fontSize: FONT_SIZE_BUTTON,
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                    cursor: "pointer",
                  }}
                >
                  ESC
                </button>
                <div className="flex flex-row items-center gap-2 ml-auto">
                  {(strip.pdc_state === "REQUESTED" || strip.pdc_state === "REQUESTED_WITH_FAULTS") && (
                    <button
                      onClick={() => {
                        revertToVoice(strip.callsign);
                        setDialogOpen(false);
                      }}
                      style={{
                        height: 70,
                        fontFamily: FONT_FAMILY,
                        fontWeight: "bold",
                        fontSize: FONT_SIZE_BUTTON,
                        backgroundColor: COLOR_REVERT_BTN,
                        color: "black",
                        padding: "4px 12px",
                        whiteSpace: "nowrap",
                      }}
                    >
                      REVERT TO VOICE
                    </button>
                  )}
                  <button
                    onClick={() => {
                      if (strip.pdc_state === "REQUESTED" || strip.pdc_state === "REQUESTED_WITH_FAULTS") {
                        clearPdc(strip.callsign, null);
                      } else {
                        moveAction(strip.callsign, Bay.Cleared);
                      }

                      setDialogOpen(false);
                    }}
                    style={{
                      width: 125,
                      height: 70,
                      backgroundColor: "#3F3F3F",
                      color: "#FFFFFF",
                      fontFamily: FONT_FAMILY,
                      fontWeight: "bold",
                      fontSize: FONT_SIZE_BUTTON,
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "center",
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
        <DialogContent className={CLS_CLX_DIALOG}>
          <VisuallyHidden.Root>
            <DialogTitle>Flight plan unavailable</DialogTitle>
          </VisuallyHidden.Root>
          <div className={CLS_CLX_PANEL}>
            <div className="bg-white px-4 py-3 text-center text-xl font-semibold">
              Flight plan unavailable
            </div>
            <p className="mt-4 text-center text-sm">
              The selected strip is no longer visible in the active bays.
            </p>
            <div className="mt-4">
              <Button variant="darkaction" className="h-12 w-full" onClick={() => setDialogOpen(false)}>
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
    </>
  );
}
