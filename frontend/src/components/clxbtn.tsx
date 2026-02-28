import {Button} from "@/components/ui/button"
import {Dialog, DialogContent, DialogTrigger,} from "@/components/ui/dialog"
import {Input} from "@/components/ui/input"
import {Label} from "@/components/ui/label"
import React, {useState} from 'react';
import StandDialog from "./stand/StandDialog";
import {useStrip, useWebSocketStore} from "@/store/store-hooks.ts";
import {Bay} from "@/api/models.ts";
import { getSimpleAircraftType } from "@/lib/utils";

const FONT_FAMILY = 'Arial';
const FONT_SIZE_FIELD = 20;   // px — bold fields
const FONT_SIZE_LABEL = 16;   // px — light labels
const FONT_SIZE_BUTTON = 24;  // px — footer buttons

function useEditableField(value: string | number | undefined | null) {
  const [fieldValue, setFieldValue] = useState(value?.toString() ?? "");
  const [focused, setFocused] = useState(false);

  const handleSetFocused = (f: boolean) => {
    if (f) setFieldValue(value?.toString() ?? "");
    setFocused(f);
  };

  // While focused show the user's input; otherwise reflect the external value
  const displayValue = focused ? fieldValue : (value?.toString() ?? "");

  return [displayValue, setFieldValue, focused, handleSetFocused] as const;
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

  const [sid, setSid, _sidFocused, setSidFocused] = useEditableField(strip?.sid);
  const [eobt, setEobt, _eobtFocused, setEobtFocused] = useEditableField(strip?.eobt);
  const [route, setRoute, _routeFocused, setRouteFocused] = useEditableField(strip?.route);
  const [hdg, setHdg, _hdgFocused, setHdgFocused] = useEditableField(strip?.heading);
  const [alt, setAlt, _altFocused, setAltFocused] = useEditableField(strip?.cleared_altitude);

  if (!strip) return null;

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="clx">{children}</Button>
      </DialogTrigger>
      <DialogContent className="bg-[#d4d4d4] rounded-none p-[25px] flex flex-col gap-0" style={{ width: 1000, maxWidth: 1000, height: 925, maxHeight: 925 }}>
        <div className="relative border-2 border-black flex-1 flex flex-col items-center gap-[30px] min-h-0" style={{ paddingTop: 30, paddingBottom: 30 }}>
          <span className="absolute font-bold text-base bg-[#d4d4d4] px-2" style={{ top: -11, left: '50%', transform: 'translateX(-50%)', whiteSpace: 'nowrap' }}>FLIGHT PLAN</span>
          {/* Row 1: C/S ADES RNAV SID SSR TTOT CTOT */}
          <div className="flex gap-[5px]" style={{ width: 835 }}>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>C/S</Label>
              <Input value={strip.callsign} disabled className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]" style={{ width: 180, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>ADES</Label>
              <Input value={strip.destination} disabled className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]" style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>RNAV</Label>
              <Input value={strip.capabilities} disabled className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]" style={{ width: 75, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>SID</Label>
              <input value={sid} onChange={e => setSid(e.target.value)} onFocus={() => setSidFocused(true)} onBlur={() => { setSidFocused(false); updateStrip(callsign, { sid }); }} onKeyDown={e => e.key === "Enter" && updateStrip(callsign, { sid })} className="border border-black rounded-none bg-[#ededed] text-black font-bold h-[50px] text-center" style={{ width: 150, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>SSR</Label>
              <Button className="border border-black rounded-none bg-[#ededed] text-black font-bold disabled:opacity-100 h-[50px] text-center select-none hover:bg-[#ededed]" style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} onClick={() => generateSquawk(callsign)}>{strip.assigned_squawk}</Button>
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>TTOT</Label>
              <input placeholder="" disabled className="border border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]" style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>CTOT</Label>
              <input value={strip.ctot} disabled className="border border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]" style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
            </div>
          </div>
          {/* Row 2: EOBT TOBT TSAT RWY ... REA */}
          <div className="flex gap-[5px] justify-between" style={{ width: 835 }}>
            <div className="flex gap-[5px]">
              <div className="grid items-center gap-[5px]">
                <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>EOBT</Label>
                <input value={eobt} onChange={e => setEobt(e.target.value)} onFocus={() => setEobtFocused(true)} onBlur={() => { setEobtFocused(false); updateStrip(callsign, { eobt }); }} onKeyDown={e => e.key === "Enter" && updateStrip(callsign, { eobt })} className="border border-black rounded-none bg-[#ededed] text-black font-bold h-[50px] text-center" style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
              </div>
              <div className="grid items-center gap-[5px]">
                <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>TOBT</Label>
                <Input value={strip.tobt} disabled className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]" style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
              </div>
              <div className="grid items-center gap-[5px]">
                <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>TSAT</Label>
                <Input value={strip.tsat} disabled className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]" style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
              </div>
              <div className="grid items-center gap-[5px]">
                <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>RWY</Label>
                <Input value={strip.runway} disabled className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]" style={{ width: 150, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
              </div>
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>REA</Label>
              <Input value={strip.release_point} disabled className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]" style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
            </div>
          </div>
          {/* Row 3: TYPE FL SPEED STS */}
          <div className="flex gap-[5px]" style={{ width: 835 }}>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>TYPE</Label>
              <Input value={getSimpleAircraftType(strip.aircraft_type)} disabled className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-left pl-2 h-[50px]" style={{ width: 200, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>FL</Label>
              <Input value={strip.requested_altitude ? Math.floor(strip.requested_altitude / 100).toString() : ""} disabled className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-left pl-2 h-[50px]" style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>SPEED</Label>
              <Input defaultValue="" disabled className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-left pl-2 h-[50px]" style={{ width: 100, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light text-center" style={{ fontSize: FONT_SIZE_LABEL }}>STS</Label>
              <Input defaultValue="" disabled className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]" style={{ width: 420, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
            </div>
          </div>
          {/* Row 4: ROUTE */}
          <div className="flex flex-col gap-[5px]" style={{ width: 835 }}>
            <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>ROUTE</Label>
            <textarea value={route} onChange={e => setRoute(e.target.value)} onFocus={() => setRouteFocused(true)} onBlur={() => { setRouteFocused(false); updateStrip(callsign, { route }); }} onKeyDown={e => e.key === "Enter" && !e.shiftKey && updateStrip(callsign, { route })} className="border border-black rounded-none bg-[#ededed] text-black font-normal text-center h-[80px] break-words resize-none w-full" style={{ fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
          </div>
          {/* Row 5: COOPANS REMARKS */}
          <div className="flex flex-col gap-[5px]" style={{ width: 835 }}>
            <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>COOPANS REMARKS</Label>
            <Input value={strip.remarks} disabled className="border-black rounded-none disabled:bg-[#9e989c] text-black font-normal disabled:opacity-100 text-center h-[50px] w-full" style={{ fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
          </div>
          {/* Row 6: NITOS REMARKS + IATA TYPE */}
          <div className="flex gap-[5px]" style={{ width: 835 }}>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>NITOS REMARKS</Label>
              <Input defaultValue="" disabled className="border-black rounded-none disabled:bg-[#9e989c] text-black font-normal disabled:opacity-100 text-center h-[50px]" style={{ width: 700, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>IATA TYPE</Label>
              <Input defaultValue="" disabled className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]" style={{ width: 130, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
            </div>
          </div>
          {/* Row 7: bottom row auto-layout */}
          <div className="flex justify-between" style={{ width: 835 }}>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>CLIMB GR.</Label>
              <Input defaultValue="" disabled className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]" style={{ width: 125, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>HDG</Label>
              <input value={hdg} onChange={e => setHdg(e.target.value)} onFocus={() => setHdgFocused(true)} onBlur={() => { setHdgFocused(false); updateStrip(callsign, { heading: hdg ? Number(hdg) : undefined }); }} onKeyDown={e => e.key === "Enter" && updateStrip(callsign, { heading: hdg ? Number(hdg) : undefined })} className="border border-black rounded-none bg-[#ededed] text-black font-bold h-[50px] text-center" style={{ width: 125, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>ALT</Label>
              <input value={alt} onChange={e => setAlt(e.target.value)} onFocus={() => setAltFocused(true)} onBlur={() => { setAltFocused(false); updateStrip(callsign, { altitude: alt ? Number(alt) : undefined }); }} onKeyDown={e => e.key === "Enter" && updateStrip(callsign, { altitude: alt ? Number(alt) : undefined })} className="border border-black rounded-none bg-[#ededed] text-black font-bold h-[50px] text-center" style={{ width: 125, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>DE-ICE</Label>
              <Input defaultValue="" disabled className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]" style={{ width: 125, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
            </div>
            <div className="grid items-center gap-[5px]">
              <Label className="font-light" style={{ fontSize: FONT_SIZE_LABEL }}>REG</Label>
              <Input defaultValue="" disabled className="border-black rounded-none disabled:bg-[#9e989c] text-black font-bold disabled:opacity-100 text-center h-[50px]" style={{ width: 125, fontFamily: FONT_FAMILY, fontSize: FONT_SIZE_FIELD }} />
            </div>
            <div style={{ width: 125 }}>
              <StandDialog value={strip.stand} onSelect={(stand) => updateStrip(callsign, { stand })} />
            </div>
          </div>
        </div>
        <div className="flex flex-row items-center justify-between pt-3">
          <a onClick={() => setOpen(false)} style={{ width: 125, height: 70, backgroundColor: '#3F3F3F', color: '#FFFFFF', fontFamily: FONT_FAMILY, fontWeight: 'bold', fontSize: FONT_SIZE_BUTTON, display: 'flex', alignItems: 'center', justifyContent: 'center', cursor: 'pointer' }}>ESC</a>
          <div className="flex flex-row items-center gap-2">
            {strip.pdc_state === "REQUESTED" && (
              <button onClick={() => clearPdc(strip.callsign, null)} style={{ fontFamily: FONT_FAMILY, fontWeight: 'bold', fontSize: FONT_SIZE_BUTTON, backgroundColor: '#3F3F3F', color: '#FFFFFF', padding: '4px 12px' }}>ISSUE PDC</button>
            )}
            {strip.pdc_state === "CONFIRMED" && (
              <button onClick={() => revertToVoice(strip.callsign)} style={{ fontFamily: FONT_FAMILY, fontWeight: 'bold', fontSize: FONT_SIZE_BUTTON, backgroundColor: '#FFFB03', color: '#000000', padding: '4px 12px', whiteSpace: 'nowrap' }}>REVERT TO VOICE</button>
            )}
            <button onClick={() => strip?.pdc_state === "REQUESTED" ? clearPdc(strip.callsign, null) : moveAction(strip.callsign, Bay.Cleared)} style={{ width: 125, height: 70, backgroundColor: '#3F3F3F', color: '#FFFFFF', fontFamily: FONT_FAMILY, fontWeight: 'bold', fontSize: FONT_SIZE_BUTTON, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>CLD</button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}


