import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { useAirport, useMetar } from "@/store/store-hooks"

export default function ATIS() {
    const airport = useAirport();
    const metar = useMetar();

    return (
        <Dialog>
        <DialogTrigger asChild>
            <button className="bg-[#646464] text-xl font-bold p-2 border-2">
                ATIS
            </button>
        </DialogTrigger>
        <DialogContent className="bg-[#e4e4e4] w-[42rem] border-4 border-primary">
          <DialogHeader>
            <DialogTitle className="text-primary font-semibold text-xl">
              METAR — {airport || "EKCH"}
            </DialogTitle>
          </DialogHeader>
          <div className="flex flex-col gap-4">
            <pre className="font-mono text-sm whitespace-pre-wrap break-words bg-black text-green-400 p-4 rounded min-h-16">
              {metar || "No METAR available"}
            </pre>
          </div>
        </DialogContent>
      </Dialog>
    )
}