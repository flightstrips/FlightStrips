import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { scalePx } from "@/lib/viewportScale";

const DIALOG_WIDTH = scalePx(500);
const FONT_SIZE = scalePx(18);
const FONT_SIZE_BUTTON = scalePx(20);

interface ManualPdcBypassDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  callsign: string;
  onConfirm: () => void;
}

export function ManualPdcBypassDialog({
  open,
  onOpenChange,
  callsign,
  onConfirm,
}: ManualPdcBypassDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        style={{
          width: DIALOG_WIDTH,
          padding: scalePx(20),
          gap: scalePx(16),
        }}
      >
        <DialogTitle className="sr-only">Manual PDC Bypass Warning</DialogTitle>
        <div className="flex flex-col gap-4">
          <div className="text-center" style={{ fontSize: FONT_SIZE, fontWeight: "bold" }}>
            <span style={{ color: "red" }}>ACTIVE PDC REQUEST</span>
            <br />
            <span style={{ color: "red" }}>This strip already has a PDC</span>
            <br />
            <span style={{ color: "red" }}>Do you want to give the clearance over voice?</span>
          </div>
          <div
            className="border border-black text-center font-bold"
            style={{ fontSize: scalePx(16), backgroundColor: "#FFD700", color: "black", padding: scalePx(10) }}
          >
            {callsign}: Manual voice clearance
          </div>
          <div className="flex justify-center gap-4">
            <Button
              onClick={onConfirm}
              style={{
                fontSize: FONT_SIZE_BUTTON,
                padding: `${scalePx(8)}px ${scalePx(24)}px`,
                backgroundColor: "#3F3F3F",
                color: "white",
              }}
            >
              YES
            </Button>
            <Button
              onClick={() => onOpenChange(false)}
              style={{
                fontSize: FONT_SIZE_BUTTON,
                padding: `${scalePx(8)}px ${scalePx(24)}px`,
                backgroundColor: "#3F3F3F",
                color: "white",
              }}
            >
              NO
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
