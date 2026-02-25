import { useState } from "react"
import {
  Dialog,
  DialogContent,
  DialogTrigger,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import StandMap from "./StandMap"

interface StandDialogProps {
  value: string;
  onSelect: (stand: string) => void;
}

export default function StandDialog({ value, onSelect }: StandDialogProps) {
  const [open, setOpen] = useState(false);

  const handleSelect = (stand: string) => {
    onSelect(stand);
    setOpen(false);
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <div>
          <Label htmlFor="stand">Stand</Label>
          <Input
            id="stand"
            value={value}
            readOnly
            className="border-black rounded-none text-black font-semibold w-full text-center cursor-pointer"
          />
        </div>
      </DialogTrigger>
      <DialogContent className="h-screen max-w-none max-h-none w-screen">
        <StandMap onSelect={handleSelect} />
      </DialogContent>
    </Dialog>
  )
}
