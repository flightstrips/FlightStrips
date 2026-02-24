import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { useMetar } from "@/hooks/use-metar"
import MetarHelper from "@/components/MetarHelper"


export default function ATIS() {
    return (
        <Dialog>
        <DialogTrigger asChild>
            <button className="bg-[#646464] text-xl font-bold p-2 border-2">
                ATIS
            </button>
        </DialogTrigger>
        <DialogContent className="bg-[#e4e4e4] w-[42rem] border-4 border-primary">
          <DialogHeader>
            <DialogTitle className="text-primary font-semibold text-xl">METAR</DialogTitle>
          </DialogHeader>
          <div className="flex flex-col  items-center justify-center">
            <div className="bg-gray-100 w-full text-center h-16 flex items-center justify-center border-primary border-2">
                <MetarHelper metar={useMetar("EKCH")} style="full" />
            </div>
            <div className="flex gap-12 pt-6">
                <section className="flex flex-col items-center">
                    <p className="font-semibold text-lg text-primary">WIND</p>
                    <p><MetarHelper metar={useMetar("EKCH")} style="winds" /></p>
                </section>
                <section className="flex flex-col items-center">
                    <p className="font-semibold text-lg text-primary">TEMOERATURE</p>
                    <p><MetarHelper metar={useMetar("EKCH")} style="temp" /></p>
                </section>
                <section className="flex flex-col items-center">
                    <p className="font-semibold text-lg text-primary">Conditions</p>
                    <p><MetarHelper metar={useMetar("EKCH")} style="conditions" /></p>
                </section>
            </div>
          </div>
          <DialogFooter>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    )
}



