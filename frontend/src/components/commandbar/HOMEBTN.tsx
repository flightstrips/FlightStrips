import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import * as VisuallyHidden from "@radix-ui/react-visually-hidden";
import { useWebSocketStore } from "@/store/store-hooks";

const EKCH_SCOPES = [
  { label: "CLR DEL", layout: "CLX" },
  { label: "AA + AD", layout: "AAAD" },
  { label: "ESET", layout: "ESET" },
  { label: "GE / GW", layout: "GEGW" },
  { label: "TW / TE", layout: "TWTE" },
];

export default function HOMEBTN() {
  const [open, setOpen] = useState(false);
  const currentLayout = useWebSocketStore((state) => state.layout);
  const displayedLayout = useWebSocketStore((state) => state.displayedLayout);
  const setDisplayedLayout = useWebSocketStore((state) => state.setDisplayedLayout);

  const handleSelect = (layout: string) => {
    setDisplayedLayout(layout);
    setOpen(false);
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <button className="bg-[#646464] text-xl font-bold p-2 border-2">
          <img src="/home.svg" width="39" height="39" alt="home icon" />
        </button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-[300px] bg-[#b3b3b3]">
        <VisuallyHidden.Root>
          <DialogTitle>Select View</DialogTitle>
        </VisuallyHidden.Root>
        <div className="border-2 border-black">
          <div className="grid grid-cols-2 gap-2 p-2" style={{ color: "#000" }}>
            {EKCH_SCOPES.map((scope) => (
              <Button
                key={scope.layout}
                variant="trf"
                className={`font-normal text-base h-fit py-3 ${
                  displayedLayout === scope.layout ? "ring-2 ring-yellow-400" : ""
                } ${
                  currentLayout === scope.layout && displayedLayout !== scope.layout ? "border-2 border-blue-500" : ""
                }`}
                onClick={() => handleSelect(scope.layout)}
              >
                {scope.label}
              </Button>
            ))}
          </div>
          <DialogFooter className="flex justify-center w-full h-14">
            <Button variant="darkaction" className="w-4/5" onClick={() => setOpen(false)}>
              ESC
            </Button>
          </DialogFooter>
        </div>
      </DialogContent>
    </Dialog>
  );
}
