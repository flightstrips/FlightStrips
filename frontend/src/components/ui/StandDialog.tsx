import {
  Dialog,
  DialogContent,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import StandMap from "../StandMap"

export default function StandDialog() {
  return (
    <Dialog>
      <DialogTrigger asChild>
        <div>
            <Label htmlFor="stand">
            Stand
            </Label>
            <Input 
            id="stand"
            defaultValue=""
            className="border-black rounded-none disabled:bg-[#9e989c] text-black font-semibold disabled:opacity-100 w-full text-center" />
        </div>
      </DialogTrigger>
      
      <DialogContent className="h-screen max-w-none max-h-none w-screen">

        <StandMap />

      </DialogContent>
    </Dialog>
  )
}
