import * as VisuallyHidden from "@radix-ui/react-visually-hidden";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import type { FrontendStrip } from "@/api/models";

interface EstDeIceDialogProps {
  open: boolean;
  strip: FrontendStrip | undefined;
  selectedPlatform?: string;
  onOpenChange: (open: boolean) => void;
  onSelectPlatform: (platform: string) => void;
  onErase: () => void;
}

const PLATFORMS = ["DE-ICE 1", "DE-ICE 2", "DE-ICE 3", "DE-ICE 4"];

// Tailwind class constants (hex must be literal strings for JIT)
const CLS_DIALOG = "w-[360px] rounded-none border border-black bg-[#B3B3B3] p-4 text-black";
const CLS_PANEL  = "border border-black bg-[#D6D6D6] p-4";

export default function EstDeIceDialog({
  open,
  strip,
  selectedPlatform,
  onOpenChange,
  onSelectPlatform,
  onErase,
}: EstDeIceDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className={CLS_DIALOG}>
        <VisuallyHidden.Root>
          <DialogTitle>De-ice</DialogTitle>
        </VisuallyHidden.Root>

        <div className={CLS_PANEL}>
          <div className="bg-white px-4 py-2 text-center text-2xl font-semibold">DE-ICE</div>
          <div className="mt-4 space-y-2 text-sm font-semibold">
            <div>
              De-Ice Operator: <span className="font-normal">{strip?.remarks || "—"}</span>
            </div>
            <div>
              Current platform: <span className="font-normal">{selectedPlatform || "—"}</span>
            </div>
            <div className="pt-2 text-base">SEM PLAT</div>
          </div>

          <div className="mt-3 grid grid-cols-2 gap-2">
            {PLATFORMS.map((platform) => (
              <Button
                key={platform}
                variant="trf"
                className={`h-12 text-sm font-semibold ${selectedPlatform === platform ? "bg-[#1BFF16] hover:bg-[#17d912]" : ""}`}
                onClick={() => onSelectPlatform(platform)}
              >
                {platform}
              </Button>
            ))}
          </div>

          <div className="mt-4 flex gap-2">
            <Button variant="darkaction" className="h-12 flex-1" onClick={onErase}>
              ERASE
            </Button>
            <Button variant="darkaction" className="h-12 flex-1" onClick={() => onOpenChange(false)}>
              ESC
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
