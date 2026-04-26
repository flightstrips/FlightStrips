import * as VisuallyHidden from "@radix-ui/react-visually-hidden";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import type { FrontendStrip } from "@/api/models";
import { scalePx } from "@/lib/viewportScale";

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
const CLS_DIALOG = "rounded-none border border-black bg-[#B3B3B3] text-black";
const CLS_PANEL  = "border border-black bg-[#D6D6D6]";

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
      <DialogContent className={CLS_DIALOG} style={{ width: scalePx(360), padding: scalePx(16) }}>
        <VisuallyHidden.Root>
          <DialogTitle>De-ice</DialogTitle>
        </VisuallyHidden.Root>

        <div className={CLS_PANEL} style={{ padding: scalePx(16) }}>
          <div className="bg-white text-center font-semibold" style={{ padding: `${scalePx(8)} ${scalePx(16)}`, fontSize: scalePx(24) }}>DE-ICE</div>
          <div className="font-semibold" style={{ marginTop: scalePx(16), display: "grid", gap: scalePx(8), fontSize: scalePx(14) }}>
            <div>
              De-Ice Operator: <span className="font-normal">{strip?.remarks || "—"}</span>
            </div>
            <div>
              Current platform: <span className="font-normal">{selectedPlatform || "—"}</span>
            </div>
            <div style={{ paddingTop: scalePx(8), fontSize: scalePx(16) }}>SEM PLAT</div>
          </div>

          <div className="grid grid-cols-2" style={{ marginTop: scalePx(12), gap: scalePx(8) }}>
            {PLATFORMS.map((platform) => (
              <Button
                key={platform}
                variant="trf"
                className={`font-semibold ${selectedPlatform === platform ? "bg-[#1BFF16] hover:bg-[#17d912]" : ""}`}
                style={{ height: scalePx(48), fontSize: scalePx(14) }}
                onClick={() => onSelectPlatform(platform)}
              >
                {platform}
              </Button>
            ))}
          </div>

          <div className="flex" style={{ marginTop: scalePx(16), gap: scalePx(8) }}>
            <Button variant="darkaction" className="flex-1" style={{ height: scalePx(48), fontSize: scalePx(24) }} onClick={onErase}>
              ERASE
            </Button>
            <Button variant="darkaction" className="flex-1" style={{ height: scalePx(48), fontSize: scalePx(24) }} onClick={() => onOpenChange(false)}>
              ESC
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
