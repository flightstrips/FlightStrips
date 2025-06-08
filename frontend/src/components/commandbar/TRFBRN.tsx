import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogTrigger,
} from "@/components/ui/dialog"

export default function TRFBRN() {
  return (
    <Dialog>
      <DialogTrigger asChild>
        <button className="bg-[#646464] text-xl font-bold p-2 border-2">
          TRF
        </button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-[425px] bg-[#b3b3b3]">
        <div className="border-2 border-black">
          <div className="w-64 h-96 grid grid-cols-2 gap-2 p-2">
            <Button type="submit" variant="trf" className="font-normal text-base p-0 m-0 h-fit py-1">A_GND <br /> 121.630</Button>
            <Button type="submit" variant="trf" className="font-normal text-base p-0 m-0 h-fit py-1">D_GND <br /> 121.730</Button>
            <Button type="submit" variant="trf" className="font-normal text-base p-0 m-0 h-fit py-1">C_TWR <br /> 118.580</Button>
            <Button type="submit" variant="trf" className="font-normal text-base p-0 m-0 h-fit py-1">GE_TWR <br /> 121.730</Button>
            <Button type="submit" variant="trf" className="font-normal text-base p-0 m-0 h-fit py-1">A_TWR <br /> 118.105</Button>
            <Button type="submit" variant="trf" className="font-normal text-base p-0 m-0 h-fit py-1">D_TWR <br /> 1119.355</Button>
            <Button type="submit" variant="trf" className="font-normal text-base p-0 m-0 h-fit py-1">R_DEP <br /> 120.255</Button>
            <Button type="submit" variant="trf" className="font-normal text-base p-0 m-0 h-fit py-1">K_DEP <br /> 124.980</Button>
            <Button type="submit" variant="trf" className="font-normal text-base p-0 m-0 h-fit py-1">W_APP <br /> 119.805</Button>
            <Button type="submit" variant="trf" className="font-normal text-base p-0 m-0 h-fit py-1">O_APP <br /> 118.455</Button>
          </div>
          <DialogFooter className="flex justify-center w-full h-14">
            <Button type="submit" variant="darkaction" className="w-4/5">ESC</Button>
          </DialogFooter>
        </div>
      </DialogContent>
    </Dialog>
  )
}


