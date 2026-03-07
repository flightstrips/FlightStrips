import * as VisuallyHidden from "@radix-ui/react-visually-hidden";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import type { FrontendStrip } from "@/api/models";

interface EsetDeIceDialogProps {
  open: boolean;
  strip: FrontendStrip | undefined;
  onOpenChange: (open: boolean) => void;
  onSelectPlatform: (platform: string) => void;
}

const PLATFORMS = ["DE-ICE 1", "DE-ICE 2", "DE-ICE 3", "DE-ICE 4"];

export default function EsetDeIceDialog({
  open,
  strip,
  onOpenChange,
  onSelectPlatform,
}: EsetDeIceDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[360px] rounded-none border border-black bg-[#B3B3B3] p-4 text-black">
        <VisuallyHidden.Root>
          <DialogTitle>De-ice</DialogTitle>
        </VisuallyHidden.Root>

        <div className="border border-black bg-[#D6D6D6] p-4">
          <div className="bg-white px-4 py-2 text-center text-2xl font-semibold">DE-ICE</div>
          <div className="mt-4 space-y-2 text-sm font-semibold">
            <div>
              De-Ice Operator: <span className="font-normal">{strip?.remarks || "—"}</span>
            </div>
            <div className="pt-2 text-base">SEM PLAT</div>
          </div>

          <div className="mt-3 grid grid-cols-2 gap-2">
            {PLATFORMS.map((platform) => (
              <Button
                key={platform}
                variant="trf"
                className="h-12 text-sm font-semibold"
                onClick={() => onSelectPlatform(platform)}
              >
                {platform}
              </Button>
            ))}
          </div>

          <div className="mt-4">
            <Button variant="darkaction" className="h-12 w-full" onClick={() => onOpenChange(false)}>
              ESC
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
