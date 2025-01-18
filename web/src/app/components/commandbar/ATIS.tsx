import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"


export default function ATIS() {
    return (
        <Dialog>
        <DialogTrigger asChild>
            <button className="bg-[#646464] text-xl font-bold p-2 border-2">
                ATIS
            </button>
        </DialogTrigger>
        <DialogContent className="bg-[#e4e4e4] w-[42rem]">
          <DialogHeader>
            <DialogTitle >METAR</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col  items-center justify-center">
            <div className="bg-[#FCFCFC] w-full text-center h-20 flex items-center justify-center">
                EKCH 181250Z 26005KT 230V290 3000 BR OVC003 05/04 Q1032 NOSIG
            </div>
            <div className="flex gap-12">
                <section className="flex flex-col items-center">
                    <p>WIND</p>
                    <p>260° 5kts</p>
                    <p className="text-xs">(230V290)</p>
                </section>
                <section className="flex flex-col items-center">
                    <p>TEMOERATURE</p>
                    <p>5°c</p>
                </section>
                <section className="flex flex-col items-center">
                    <p>Conditions</p>
                    <p>Fog</p>
                </section>
            </div>
          </div>
          <DialogFooter>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    )
}


