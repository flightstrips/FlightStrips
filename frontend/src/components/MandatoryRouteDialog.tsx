import React from "react";
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
  routes: string[];
  onConfirm: () => void;
  onCancel: () => void;
}

export function MandatoryRouteDialog({
  open,
  onOpenChange,
  callsign,
  routes,
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
            <span style={{ color: "red" }}>Would you like</span>
            <br />
            <span style={{ color: "red" }}>MANDATORY ROUTE</span>
            <br />
            <span style={{ color: "red" }}>to be sent via PM or PDC?</span>
          </div>
          {routes.length > 0 && (
            <div className="text-center" style={{ fontSize: scalePx(14), color: "#888" }}>
              {callsign}: {routes.join(" | ")}
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
              YES
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
              NO
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}