import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { scalePx } from "@/lib/viewportScale";

const DIALOG_WIDTH = scalePx(485);
const INNER_PADDING = scalePx(20);
const BUTTON_WIDTH = scalePx(191);
const BUTTON_HEIGHT = scalePx(70);
const BUTTON_GAP = scalePx(21);

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
  callsign: _callsign,
  route: _route,
  filedSid: _filedSid,
  mandatorySid: _mandatorySid,
  sidMismatch: _sidMismatch,
  pdcRequested: _pdcRequested,
  onConfirm,
  onCancel,
}: MandatoryRouteDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="border border-black rounded-none"
        style={{
          width: DIALOG_WIDTH,
          maxWidth: DIALOG_WIDTH,
          padding: 0,
          gap: 0,
          backgroundColor: "#E4E4E4",
        }}
      >
        <DialogTitle className="sr-only">Mandatory Route Confirmation</DialogTitle>
        <div
          className="border border-black m-[20px] flex flex-col items-center"
          style={{
            paddingTop: scalePx(33),
            paddingBottom: scalePx(29),
            paddingInline: INNER_PADDING,
            color: "black",
          }}
        >
          <div
            style={{
              fontFamily: "Rubik, Arial, sans-serif",
              fontSize: scalePx(32),
              fontWeight: 400,
              color: "#FF0000",
              textAlign: "center",
              lineHeight: 1.2,
              marginBottom: scalePx(19),
            }}
          >
            Would you like
            <br />
            MANDATORY ROUTE
            <br />
            to be send via PM or PDC?
          </div>
          <div className="flex" style={{ gap: BUTTON_GAP }}>
            <button
              type="button"
              onClick={onCancel}
              style={{
                width: BUTTON_WIDTH,
                height: BUTTON_HEIGHT,
                backgroundColor: "#3F3F3F",
                color: "white",
                fontFamily: "Rubik, Arial, sans-serif",
                fontSize: scalePx(32),
                fontWeight: 600,
                textAlign: "center",
                border: "none",
                cursor: "pointer",
              }}
            >
              NO
            </button>
            <button
              type="button"
              onClick={onConfirm}
              style={{
                width: BUTTON_WIDTH,
                height: BUTTON_HEIGHT,
                backgroundColor: "#3F3F3F",
                color: "white",
                fontFamily: "Rubik, Arial, sans-serif",
                fontSize: scalePx(32),
                fontWeight: 600,
                textAlign: "center",
                border: "none",
                cursor: "pointer",
              }}
            >
              YES
            </button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
