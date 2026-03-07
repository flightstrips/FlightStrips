import { useState } from "react"
import {
  Dialog,
  DialogContent,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import * as VisuallyHidden from "@radix-ui/react-visually-hidden"
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
        <div className="grid items-center gap-[5px]">
          <Label htmlFor="stand" className="font-light text-[16px]">Stand</Label>
          <Input
            id="stand"
            value={value}
            readOnly
            className="border-black rounded-none text-black font-bold text-[18px] w-full text-center cursor-pointer h-[50px]"
            style={{ fontFamily: 'Arial' }}
          />
        </div>
      </DialogTrigger>
      <DialogContent className="h-screen max-w-none max-h-none w-screen">
        <VisuallyHidden.Root>
          <DialogTitle>Select Stand</DialogTitle>
        </VisuallyHidden.Root>
        <StandMap onSelect={handleSelect} />
      </DialogContent>
    </Dialog>
  )
}
