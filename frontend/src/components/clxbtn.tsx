import {Button} from "@/components/ui/button"
import {Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger,} from "@/components/ui/dialog"
import {Input} from "@/components/ui/input"
import {Label} from "@/components/ui/label"
import React, {useEffect, useState} from 'react';
import StandDialog from "./stand/StandDialog";
import {useStrip, useWebSocketStore} from "@/store/store-hooks.ts";
import {Bay} from "@/api/models.ts";

function useEditableField(value: string | number | undefined | null) {
  const [fieldValue, setFieldValue] = useState(value?.toString() ?? "");
  const [focused, setFocused] = useState(false);
  useEffect(() => {
    if (!focused) setFieldValue(value?.toString() ?? "");
  }, [value, focused]);
  return [fieldValue, setFieldValue, focused, setFocused] as const;
}

export function CLXBtn({ callsign, children }: { callsign: string, children?: React.ReactNode }) {
  // TODO Simon this is very hacky due to where this component is place (a.k.a in the button of the component tree)
  const strip = useStrip(callsign);
  const moveAction = useWebSocketStore(state => state.move);
  const generateSquawk = useWebSocketStore(state => state.generateSquawk);
  const clearPdc = useWebSocketStore(state => state.issuePdcClearance);
  const revertToVoice = useWebSocketStore(state => state.revertToVoice);
  const updateStrip = useWebSocketStore(state => state.updateStrip);

  // Controlled open state prevents the dialog from closing on re-renders triggered by strip updates
  const [open, setOpen] = useState(false);

  const [sid, setSid, sidFocused, setSidFocused] = useEditableField(strip?.sid);
  const [eobt, setEobt, eobtFocused, setEobtFocused] = useEditableField(strip?.eobt);
  const [route, setRoute, routeFocused, setRouteFocused] = useEditableField(strip?.route);
  const [hdg, setHdg, hdgFocused, setHdgFocused] = useEditableField(strip?.heading);
  const [alt, setAlt, altFocused, setAltFocused] = useEditableField(strip?.cleared_altitude);

  if (!strip) return null;

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="clx">{children}</Button>
      </DialogTrigger>
      <DialogContent className="bg-[#d4d4d4] rounded-none w-full max-w-2xl">
        <DialogHeader>
          <DialogTitle>FLIGHT PLAN</DialogTitle>
        </DialogHeader>
        <div className="flex gap-2">
          <div className="grid items-center">
            <Label>C/S</Label>
            <Input
              value={strip.callsign}
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-32 text-center"
            />
          </div>
          <div className="grid items-center">
            <Label>ADES</Label>
            <Input
              value={strip.destination}
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-20 text-center"
            />
          </div>
          <div className="grid items-center">
            <Label>RNAV</Label>
            <Input
              value={strip.capabilities}
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-16 text-center"
            />
          </div>
          <div className="grid items-center">
            <Label>SID</Label>
            <input
              value={sid}
              onChange={e => setSid(e.target.value)}
              onFocus={() => setSidFocused(true)}
              onBlur={() => { setSidFocused(false); updateStrip(callsign, { sid }); }}
              onKeyDown={e => e.key === "Enter" && updateStrip(callsign, { sid })}
              className="border-black border rounded-none bg-[#ededed] text-black font-semibold w-28 h-10 text-center"
            />
          </div>
          <div className="grid items-center">
            <Label>SSR</Label>
            <Button
              className="border-black border rounded-none bg-[#ededed] text-black font-semibold disabled:opacity-100 w-16 h-10 text-center text-lg select-none hover:bg-[#ededed]"
              onClick={() => generateSquawk(callsign)}
            >{strip.assigned_squawk}</Button>
          </div>
          <div className="grid items-center">
            <Label>TTOT</Label>
            <input
              placeholder=""
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-16 text-center h-10"
            />
          </div>
          <div className="grid items-center">
            <Label>CTOT</Label>
            <input
              value={strip.ctot}
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-16 text-center h-10"
            />
          </div>
        </div>
        <div className="flex gap-2 justify-between">
          <div className="flex gap-2">
            <div className="grid items-center">
              <Label>EOBT</Label>
              <input
                value={eobt}
                onChange={e => setEobt(e.target.value)}
                onFocus={() => setEobtFocused(true)}
                onBlur={() => { setEobtFocused(false); updateStrip(callsign, { eobt }); }}
                onKeyDown={e => e.key === "Enter" && updateStrip(callsign, { eobt })}
                className="border-black border rounded-none bg-[#ededed] text-black font-semibold w-20 h-10 text-center"
              />
            </div>
            <div className="grid items-center">
              <Label>TOBT</Label>
              <Input
                value={strip.tobt}
                disabled
                className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-20 text-center"
              />
            </div>
            <div className="grid items-center">
              <Label>TSAT</Label>
              <Input
                value={strip.tsat}
                disabled
                className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-20 text-center"
              />
            </div>
            <div className="grid items-center ml-4">
              <Label>RWY</Label>
              <Input
                value={strip.runway}
                disabled
                className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-24 text-center"
              />
            </div>
          </div>
          <div className="grid items-center">
            <Label>REA</Label>
            <Input
              value={strip.release_point}
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-20 text-center"
            />
          </div>
        </div>
        <div className="flex gap-2 w-full">
          <div className="grid items-center">
            <Label>TYPE</Label>
            <Input
              value={strip.aircraft_type}
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-28 text-left pl-2"
            />
          </div>
          <div className="grid items-center">
            <Label>FL</Label>
            <Input
              value={strip.requested_altitude ? Math.floor(strip.requested_altitude / 100).toString() : ""}
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-20 text-left pl-2"
            />
          </div>
          <div className="grid items-center">
            <Label>SPEED</Label>
            <Input
              defaultValue=""
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-20 text-left pl-2"
            />
          </div>
          <div className="grid items-center w-full">
            <Label className="text-center">STS</Label>
            <Input
              defaultValue=""
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-full text-center"
            />
          </div>
        </div>
        <div className="flex gap-2 w-full">
          <div className="grid items-center w-full">
            <Label className="text-left">ROUTE</Label>
            <textarea
              value={route}
              onChange={e => setRoute(e.target.value)}
              onFocus={() => setRouteFocused(true)}
              onBlur={() => { setRouteFocused(false); updateStrip(callsign, { route }); }}
              onKeyDown={e => e.key === "Enter" && !e.shiftKey && updateStrip(callsign, { route })}
              className="border-black border rounded-none bg-[#ededed] text-black font-semibold w-full text-center h-32 break-words resize-none"
            />
          </div>
        </div>
        <div className="flex gap-2 w-full">
          <div className="grid items-center w-full">
            <Label>COOPANS REMARKS</Label>
            <Input
              value={strip.remarks}
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-full text-center"
            />
          </div>
        </div>
        <div className="flex gap-2 w-full">
          <div className="grid items-center w-full">
            <Label>NITOS REMARKS</Label>
            <Input
              defaultValue=""
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-full text-center"
            />
          </div>
          <div className="grid items-center">
            <Label>IATA TYPE</Label>
            <Input
              defaultValue=""
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-full text-center"
            />
          </div>
        </div>
        <div className="flex gap-2 w-full">
          <div className="grid items-center">
            <Label>CLIMB GR.</Label>
            <Input
              defaultValue=""
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-full text-center"
            />
          </div>
          <div className="grid items-center">
            <Label>HDG</Label>
            <input
              value={hdg}
              onChange={e => setHdg(e.target.value)}
              onFocus={() => setHdgFocused(true)}
              onBlur={() => { setHdgFocused(false); updateStrip(callsign, { heading: hdg }); }}
              onKeyDown={e => e.key === "Enter" && updateStrip(callsign, { heading: hdg })}
              className="border-black border rounded-none bg-[#ededed] text-black font-semibold w-full h-10 text-center"
            />
          </div>
          <div className="grid items-center">
            <Label>ALT</Label>
            <input
              value={alt}
              onChange={e => setAlt(e.target.value)}
              onFocus={() => setAltFocused(true)}
              onBlur={() => { setAltFocused(false); updateStrip(callsign, { altitude: alt }); }}
              onKeyDown={e => e.key === "Enter" && updateStrip(callsign, { altitude: alt })}
              className="border-black border rounded-none bg-[#ededed] text-black font-semibold w-full h-10 text-center"
            />
          </div>
          <div className="grid items-center">
            <Label>DE-ICE</Label>
            <Input
              defaultValue=""
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-full text-center"
            />
          </div>
          <div className="grid items-center">
            <Label>REG</Label>
            <Input
              defaultValue=""
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-full text-center"
            />
          </div>
          <div className="grid items-center">
            <StandDialog
              value={strip.stand}
              onSelect={(stand) => updateStrip(callsign, { stand })}
            />
          </div>
        </div>
        <DialogFooter>
          <a type="submit">ESC</a>
          {strip.pdc_state === "REQUESTED" && (
            <button onClick={() => clearPdc(strip.callsign, null)}>ISSUE PDC</button>
          )}
          {strip.pdc_state === "CONFIRMED" && (
            <button onClick={() => revertToVoice(strip.callsign)}>REVERT TO VOICE</button>
          )}
          <button onClick={() => strip?.pdc_state === "REQUESTED" ? clearPdc(strip.callsign, null) : moveAction(strip.callsign, Bay.Cleared)}>CLD</button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
