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
import { CLS_CMDBTN } from "@/components/strip/shared";

const CLS_DIALOG = "sm:max-w-[300px] bg-[#b3b3b3]"; // active-header bg

const EKCH_SCOPES = [
  { label: "CLR DEL", layout: "CLX" },
  { label: "AA + AD", layout: "AAAD" },
  { label: "APRON ARR", layout: "AA" },
  { label: "APRON DEP", layout: "AD" },
  { label: "ESET", layout: "ESET" },
  { label: "GE / GW", layout: "GEGW" },
  { label: "TW / TE", layout: "TWTE" },
];

export default function HOMEBTN() {
  const open = useWebSocketStore((state) => state.layoutChooserOpen);
  const setOpen = useWebSocketStore((state) => state.setLayoutChooserOpen);
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
        <button className={`${CLS_CMDBTN} !w-auto px-3`}>
          <img src="/home.svg" className="h-[calc(4.72vh-20px)] w-auto" alt="home icon" />
        </button>
      </DialogTrigger>
      <DialogContent className={CLS_DIALOG}>
        <VisuallyHidden.Root>
          <DialogTitle>Select View</DialogTitle>
        </VisuallyHidden.Root>
        <div className="border-2 border-black">
          <div className="grid grid-cols-2 gap-2 p-2" style={{ color: "black" }}>
            {EKCH_SCOPES.map((scope) => (
              <Button
                key={scope.layout}
                variant="trf"
                className={`font-normal text-base h-fit py-3 ${
                  displayedLayout === scope.layout ? "ring-2 ring-yellow-400" : ""
                } ${
                  currentLayout === scope.layout && displayedLayout !== scope.layout ? "border-2 border-primary" : ""
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
