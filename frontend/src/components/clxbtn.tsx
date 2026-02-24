import {Button} from "@/components/ui/button"
import {Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger,} from "@/components/ui/dialog"
import {Input} from "@/components/ui/input"
import {Label} from "@/components/ui/label"
import React from 'react';
import StandDialog from "./stand/StandDialog";
import {useStrip, useWebSocketStore} from "@/store/store-hooks.ts";
import {Bay} from "@/api/models.ts";

export function CLXBtn({ callsign, children }: { callsign: string, children?: React.ReactNode }) {
  // TODO Simon this is very hacky due to where this component is place (a.k.a in the button of the component tree)
  const strip = useStrip(callsign);
  const moveAction = useWebSocketStore(state => state.move);
  const generateSquawk = useWebSocketStore(state => state.generateSquawk);
  const clearPdc = useWebSocketStore(state => state.issuePdcClearance);

  if (!strip) return null;

  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button variant="clx">{children}</Button>
      </DialogTrigger>
      <DialogContent className="bg-[#d4d4d4] rounded-none w-full max-w-2xl">
        <DialogHeader>
          <DialogTitle>FLIGHT PLAN</DialogTitle>
        </DialogHeader>
        <div className="flex gap-2">
          <div className="grid items-center">
            <Label htmlFor="name">
              C/S
            </Label>
            <Input
              id="name"
              defaultValue="NSZ3676"
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-32 text-center"
            />
          </div>
          <div className="grid items-center">
            <Label htmlFor="name">
              ADES
            </Label>
            <Input
              id="name"
              defaultValue="EKYT"
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-20 text-center"
            />
          </div>
          <div className="grid items-center">
            <Label htmlFor="name">
              RNAV
            </Label>
            <Input
              id="name"
              defaultValue=""
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-16 text-center"
            />
          </div>
          <div className="grid items-center">
            <Label htmlFor="name">
              SID
            </Label>
            <Button
              id="name"
              className="border-black border rounded-none bg-[#ededed] text-black font-semibold disabled:opacity-100 w-28 h-10 text-center text-lg select-none hover:bg-[#ededed]"
            >GOLGA2C</Button>
          </div>
          <div className="grid items-center">
            <Label htmlFor="name">
              SSR
            </Label>
            <Button
              id="name"
              className="border-black border rounded-none bg-[#ededed] text-black font-semibold disabled:opacity-100 w-16 h-10 text-center text-lg select-none hover:bg-[#ededed]"
              onClick={() => generateSquawk(callsign)}
            >{strip.assigned_squawk}</Button>
          </div>
          <div className="grid items-center">
            <Label htmlFor="name">
              TTOT
            </Label>
            <input
              id="name"
              placeholder=""
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-16 text-center h-10 "
            />
          </div>
          <div className="grid items-center">
            <Label htmlFor="name">
                CTOT
            </Label>
            <input
              id="name"
              placeholder=""
              disabled
              className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-16 text-center h-10 "
            />
          </div>
        </div>
        <div className="flex gap-2 justify-between">
            <div className="flex gap-2">
                <div className="grid items-center">
                    <Label htmlFor="name">
                    EOBT
                    </Label>
                    <Button
                    id="name"
                    className="border-black border rounded-none bg-[#ededed] text-black font-semibold disabled:opacity-100 w-20 h-10 text-center text-lg select-none hover:bg-[#ededed]"
                    ></Button>
                </div>
                <div className="grid items-center">
                    <Label htmlFor="name">
                    TOBT
                    </Label>
                    <Input
                    id="name"
                    defaultValue="1312"
                    disabled
                    className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-20 text-center"
                    />
                </div>
                <div className="grid items-center ">
                    <Label htmlFor="name">
                    TSAT
                    </Label>
                    <Input
                    id="name"
                    defaultValue="1312"
                    disabled
                    className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-20 text-center"
                    />
                </div>
                <div className="grid items-center ml-4" >
                    <Label htmlFor="name">
                    RWY
                    </Label>
                    <Input
                    id="name"
                    defaultValue="1312"
                    disabled
                    className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-24 text-center"
                    />
                </div>
            </div>
            <div className="grid items-center">
                <Label htmlFor="name">
                REA
                </Label>
                <Input
                id="name"
                defaultValue="1312"
                disabled
                className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-20 text-center"
                />
            </div>
        </div>
        <div className="flex gap-2 w-full">
            <div className="grid items-center">
                    <Label htmlFor="name">
                    TYPE
                    </Label>
                    <Input
                    id="name"
                    defaultValue="B738"
                    disabled
                    className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-28 text-left pl-2"
                    />
            </div>
            <div className="grid items-center">
                    <Label htmlFor="name">
                    FL
                    </Label>
                    <Input
                    id="name"
                    defaultValue="360"
                    disabled
                    className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-20 text-left pl-2"
                    />
            </div>
            <div className="grid items-center">
                    <Label htmlFor="name">
                    SPEED
                    </Label>
                    <Input
                    id="name"
                    defaultValue="450"
                    disabled
                    className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-20 text-left pl-2"
                    />
            </div>
            <div className="grid items-center w-full">
                    <Label htmlFor="name" className="text-center">
                    STS
                    </Label>
                    <Input
                    id="name"
                    defaultValue="450"
                    disabled
                    className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-full text-center"
                    />
            </div>
        </div>
        <div className="flex gap-2 w-full">
            <div className="grid items-center w-full">
                    <Label htmlFor="name" className="text-left">
                    ROUTE
                    </Label>
                    <Input
                    id="name"
                    defaultValue="NEXEN T503 GIMRU DCT MICOS DCT RIMET/N0481F390 T157 ODIPI/N0454F210 T157 KERAX KERAX4D N0454F210 T157 KERAX KERAX4DN0454F210 T157 KERAX KERAX4D"
                    className="border-black rounded-none bg-[#ededed] text-black font-semibold disabled:opacity-100 w-full text-center h-32 break-words line-clamp-2"
                    />
            </div>
        </div>
        <div className="flex gap-2 w-full">
            <div className="grid items-center w-full">
                    <Label htmlFor="name">
                    COOPANS REMARKS
                    </Label>
                    <Input
                    id="name"
                    defaultValue="450"
                    disabled
                    className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-full text-center"
                    />
            </div>
        </div>
        <div className="flex gap-2 w-full">
            <div className="grid items-center w-full">
                    <Label htmlFor="name">
                    NITOS REMARKS
                    </Label>
                    <Input
                    id="name"
                    defaultValue="450"
                    disabled
                    className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-full text-center"
                    />
            </div>
            <div className="grid items-center">
                    <Label htmlFor="name">
                    IATA TYPE
                    </Label>
                    <Input
                    id="name"
                    defaultValue="450"
                    disabled
                    className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-full text-center"
                    />
            </div>
        </div>
        <div className="flex gap-2 w-full">
          <div className="grid items-center">
                    <Label htmlFor="name">
                    CLIMB GR.
                    </Label>
                    <Input
                    id="name"
                    defaultValue=""
                    disabled
                    className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-full text-center"
                    />
            </div>
            <div className="grid items-center">
                    <Label htmlFor="name">
                    HDG
                    </Label>
                    <Input
                    id="name"
                    defaultValue=""
                    disabled
                    className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-full text-center"
                    />
            </div>
            <div className="grid items-center">
                    <Label htmlFor="name">
                    ALT
                    </Label>
                    <Input
                    id="name"
                    defaultValue=""
                    disabled
                    className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-full text-center"
                    />
            </div>
            <div className="grid items-center">
                    <Label htmlFor="name">
                    DE-ICE
                    </Label>
                    <Input
                    id="name"
                    defaultValue=""
                    disabled
                    className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-full text-center"
                    />
            </div>
            <div className="grid items-center">
                    <Label htmlFor="name">
                    REG
                    </Label>
                    <Input
                    id="name"
                    defaultValue=""
                    disabled
                    className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-full text-center"
                    />
            </div>
            <div className="grid items-center">
                    <StandDialog />
            </div>
        </div>
        <DialogFooter>
            <a type="submit">ESC</a>
            <button onClick={() => strip?.pdc_state == "REQUESTED" ?  clearPdc(strip.callsign, null) : moveAction(strip.callsign, Bay.Cleared)}>CLD</button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

