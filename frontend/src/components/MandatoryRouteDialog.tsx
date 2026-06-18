import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { scalePx } from "@/lib/viewportScale";

const DIALOG_WIDTH = scalePx(500);
const FONT_SIZE = scalePx(18);
const FONT_SIZE_BUTTON = scalePx(20);

interface MandatoryRouteDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  callsign: string;
  route: string;
  filedSid?: string;
  mandatorySid?: string;
  sidMismatch: boolean;
  pdcRequested: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}

export function MandatoryRouteDialog({
  open,
  onOpenChange,
  callsign,
  route,
  filedSid,
  mandatorySid,
  sidMismatch,
  pdcRequested,
  onConfirm,
  onCancel,
}: MandatoryRouteDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        style={{
          width: DIALOG_WIDTH,
          padding: scalePx(20),
          gap: scalePx(16),
        }}
      >
        <DialogTitle className="sr-only">Mandatory Route Confirmation</DialogTitle>
        <div className="flex flex-col gap-4">
          <div className="text-center" style={{ fontSize: FONT_SIZE, fontWeight: "bold" }}>
            <span style={{ color: "red" }}>MANDATORY ROUTE</span>
            <br />
            <span style={{ color: "red" }}>will be sent on clearance</span>
            <br />
            <span style={{ color: "red" }}>{pdcRequested ? "via PDC" : "via private message"}</span>
          </div>
          <div
            className="border border-black text-center font-bold"
            style={{ fontSize: scalePx(16), backgroundColor: "#FFD700", color: "black", padding: scalePx(10) }}
          >
            {callsign}: {route || "NO MANDATORY ROUTE"}
          </div>
          {mandatorySid && (
            <div className="flex flex-col gap-2 text-center" style={{ fontSize: scalePx(14) }}>
              <div
                className="border border-black"
                style={{
                  backgroundColor: sidMismatch ? "#FF0000" : "#B3B3B3",
                  color: sidMismatch ? "white" : "black",
                  padding: scalePx(8),
                  fontWeight: "bold",
                }}
              >
                FILED SID: {filedSid || "NONE"}
              </div>
              <div className="border border-black bg-[#D6D6D6] text-black font-bold" style={{ padding: scalePx(8) }}>
                MANDATORY SID: {mandatorySid}
              </div>
            </div>
          )}
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
              CLR
            </Button>
            <Button
              onClick={onCancel}
              style={{
                fontSize: FONT_SIZE_BUTTON,
                padding: `${scalePx(8)}px ${scalePx(24)}px`,
                backgroundColor: "#3F3F3F",
                color: "white",
              }}
            >
              CANCEL
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
