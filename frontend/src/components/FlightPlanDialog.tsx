import React, { useState } from "react";
import * as VisuallyHidden from "@radix-ui/react-visually-hidden";

import { Bay } from "@/api/models.ts";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { getSimpleAircraftType } from "@/lib/utils";
import StandDialog from "@/components/stand/StandDialog";
import { useStrip, useWebSocketStore } from "@/store/store-hooks.ts";

const FONT_FAMILY = "Arial";
const FONT_SIZE_FIELD = 20;
const FONT_SIZE_LABEL = 16;
const FONT_SIZE_BUTTON = 24;

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
}

export default function FlightPlanDialog({
  callsign,
  open,
  onOpenChange,
  children,
}: FlightPlanDialogProps) {
  const strip = useStrip(callsign);
  const moveAction = useWebSocketStore((state) => state.move);
  const generateSquawk = useWebSocketStore((state) => state.generateSquawk);
  const clearPdc = useWebSocketStore((state) => state.issuePdcClearance);
  const revertToVoice = useWebSocketStore((state) => state.revertToVoice);
  const updateStrip = useWebSocketStore((state) => state.updateStrip);

  const [internalOpen, setInternalOpen] = useState(false);
  const dialogOpen = open ?? internalOpen;
  const setDialogOpen = onOpenChange ?? setInternalOpen;

  const [sid, setSid, _sidFocused, setSidFocused] = useEditableField(strip?.sid);
  const [eobt, setEobt, _eobtFocused, setEobtFocused] = useEditableField(strip?.eobt);
  const [route, setRoute, _routeFocused, setRouteFocused] = useEditableField(strip?.route);
  const [hdg, setHdg, _hdgFocused, setHdgFocused] = useEditableField(strip?.heading);
  const [alt, setAlt, _altFocused, setAltFocused] = useEditableField(strip?.cleared_altitude);

  return (
    <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
      {children ? <DialogTrigger asChild>{children}</DialogTrigger> : null}
      {strip ? (
        <DialogContent
          className="bg-[#d4d4d4] rounded-none p-[25px] flex flex-col gap-0"
          style={{ width: 1000, maxWidth: 1000, height: 925, maxHeight: 925 }}
        >
          <VisuallyHidden.Root>
            <DialogTitle>Flight plan</DialogTitle>
          </VisuallyHidden.Root>
          <div
            className="relative border-2 border-black flex-1 flex flex-col items-center gap-[30px] min-h-0"
            style={{ paddingTop: 30, paddingBottom: 30, color: "#000" }}
          >
          <span
            className="absolute font-bold text-base bg-[#d4d4d4] px-2"
            style={{ top: -11, left: "50%", transform: "translateX(-50%)", whiteSpace: "nowrap" }}
          >
            FLIGHT PLAN
          </span>

          <div className="flex gap-[5px]" style={{ width: 835 }}>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>C/S</Label>
              <Input
                value={strip.callsign}
                disabled
                className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]"
                style={{ width: 180, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>ADES</Label>
              <Input
                value={strip.destination}
                disabled
                className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]"
                style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>RNAV</Label>
              <Input
                value={strip.capabilities}
                disabled
                className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]"
                style={{ width: 75, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>SID</Label>
              <input
                value={sid}
                onChange={(event) => setSid(event.target.value)}
                onFocus={() => setSidFocused(true)}
                onBlur={() => {
                  setSidFocused(false);
                  updateStrip(callsign, { sid });
                }}
                onKeyDown={(event) => event.key === "Enter" && updateStrip(callsign, { sid })}
                className="border border-black rounded-none bg-[#ededed] text-black font-bold h-[50px] text-center"
                style={{ width: 150, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>SSR</Label>
              <Button
                className="border border-black rounded-none bg-[#ededed] text-black font-bold disabled:opacity-100 h-[50px] text-center select-none hover:bg-[#ededed]"
                style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
                onClick={() => generateSquawk(callsign)}
              >
                {strip.assigned_squawk}
              </Button>
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>TTOT</Label>
              <input
                placeholder=""
                disabled
                className="border border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]"
                style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>CTOT</Label>
              <input
                value={strip.ctot}
                disabled
                className="border border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]"
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
                  className="border border-black rounded-none bg-[#ededed] text-black font-bold h-[50px] text-center"
                  style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
                />
              </div>
              <div className="grid items-center gap-[5px]">
                <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>TOBT</Label>
                <Input
                  value={strip.tobt}
                  disabled
                  className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]"
                  style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
                />
              </div>
              <div className="grid items-center gap-[5px]">
                <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>TSAT</Label>
                <Input
                  value={strip.tsat}
                  disabled
                  className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]"
                  style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
                />
              </div>
              <div className="grid items-center gap-[5px]">
                <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>RWY</Label>
                <Input
                  value={strip.runway}
                  disabled
                  className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]"
                  style={{ width: 150, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
                />
              </div>
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>REA</Label>
              <Input
                value={strip.release_point}
                disabled
                className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]"
                style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
          </div>

          <div className="flex gap-[5px]" style={{ width: 835 }}>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>TYPE</Label>
              <Input
                value={getSimpleAircraftType(strip.aircraft_type)}
                disabled
                className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-left pl-2 h-[50px]"
                style={{ width: 200, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>FL</Label>
              <Input
                value={strip.requested_altitude ? Math.floor(strip.requested_altitude / 100).toString() : ""}
                disabled
                className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-left pl-2 h-[50px]"
                style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>SPEED</Label>
              <Input
                defaultValue=""
                disabled
                className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-left pl-2 h-[50px]"
                style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light text-center" style={{ fontSize: FONT_SIZE_LABEL }}>STS</Label>
              <Input
                defaultValue=""
                disabled
                className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]"
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
              className="border border-black rounded-none bg-[#ededed] text-black font-normal text-center h-[80px] break-words resize-none w-full"
              style={{ fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
            />
          </div>

          <div className="flex flex-col gap-[5px]" style={{ width: 835 }}>
            <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>COOPANS REMARKS</Label>
            <Input
              value={strip.remarks}
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-normal disabled:opacity-100 text-center h-[50px] w-full"
              style={{ fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
            />
          </div>

          <div className="flex gap-[5px]" style={{ width: 835 }}>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>NITOS REMARKS</Label>
              <Input
                defaultValue=""
                disabled
                className="border-black rounded-none disabled:bg-[#9e989c] text-black font-normal disabled:opacity-100 text-center h-[50px]"
                style={{ width: 700, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>IATA TYPE</Label>
              <Input
                defaultValue=""
                disabled
                className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]"
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
                className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]"
                style={{ width: 125, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>HDG</Label>
              <input
                value={hdg}
                onChange={(event) => setHdg(event.target.value)}
                onFocus={() => setHdgFocused(true)}
                onBlur={() => {
                  setHdgFocused(false);
                  updateStrip(callsign, { heading: hdg ? Number(hdg) : undefined });
                }}
                onKeyDown={(event) => event.key === "Enter" && updateStrip(callsign, { heading: hdg ? Number(hdg) : undefined })}
                className="border border-black rounded-none bg-[#ededed] text-black font-bold h-[50px] text-center"
                style={{ width: 125, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>ALT</Label>
              <input
                value={alt}
                onChange={(event) => setAlt(event.target.value)}
                onFocus={() => setAltFocused(true)}
                onBlur={() => {
                  setAltFocused(false);
                  updateStrip(callsign, { altitude: alt ? Number(alt) : undefined });
                }}
                onKeyDown={(event) => event.key === "Enter" && updateStrip(callsign, { altitude: alt ? Number(alt) : undefined })}
                className="border border-black rounded-none bg-[#ededed] text-black font-bold h-[50px] text-center"
                style={{ width: 125, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>DE-ICE</Label>
              <Input
                defaultValue=""
                disabled
                className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]"
                style={{ width: 125, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>REG</Label>
              <Input
                defaultValue=""
                disabled
                className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]"
                style={{ width: 125, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }}
              />
            </div>
            <div style={{ width: 125 }}>
              <StandDialog value={strip.stand} onSelect={(stand) => updateStrip(callsign, { stand })} />
            </div>
          </div>
        </div>

          <div className="flex flex-row items-center justify-between pt-3">
            <button
              onClick={() => setDialogOpen(false)}
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
                cursor: "pointer",
              }}
            >
              ESC
            </button>
            <div className="flex flex-row items-center gap-2">
              {strip.pdc_state === "REQUESTED" && (
                <button
                  onClick={() => {
                    revertToVoice(strip.callsign);
                    setDialogOpen(false);
                  }}
                  style={{
                    fontFamily: FONT_FAMILY,
                    fontWeight: "bold",
                    fontSize: FONT_SIZE_BUTTON,
                    backgroundColor: "#FFFB03",
                    color: "#000000",
                    padding: "4px 12px",
                    whiteSpace: "nowrap",
                  }}
                >
                  REVERT TO VOICE
                </button>
              )}
              <button
                onClick={() => {
                  if (strip.pdc_state === "REQUESTED") {
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
          </div>
        </DialogContent>
      ) : (
        <DialogContent className="w-[360px] rounded-none border border-black bg-[#B3B3B3] p-4 text-black">
          <VisuallyHidden.Root>
            <DialogTitle>Flight plan unavailable</DialogTitle>
          </VisuallyHidden.Root>
          <div className="border border-black bg-[#D6D6D6] p-4">
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
  );
}
