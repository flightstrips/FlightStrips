import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";
import { useWebSocketStore } from "@/store/store-hooks";
import type { ValidationStatus } from "@/api/models";

const CLS_DIALOG_BG =
  "bg-[#B3B3B3] border border-black p-0 w-[360px] max-w-none max-h-none gap-0 overflow-hidden [&>button]:hidden";

interface ValidationStatusDialogProps {
  callsign: string;
  status: ValidationStatus;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function ValidationStatusDialog({
  callsign,
  status,
  open,
  onOpenChange,
}: ValidationStatusDialogProps) {
  const acknowledgeValidationStatus = useWebSocketStore((state) => state.acknowledgeValidationStatus);
  const generateSquawk = useWebSocketStore((state) => state.generateSquawk);

  function handleAcknowledge() {
    acknowledgeValidationStatus(callsign, status.activation_key);
    onOpenChange(false);
  }

  function handleCustomAction() {
    if (status.custom_action?.action_kind === "generate_squawk") {
      generateSquawk(callsign);
    }
    onOpenChange(false);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className={CLS_DIALOG_BG}>
        <DialogTitle className="sr-only">Validation Status</DialogTitle>
        <div className="flex flex-col gap-0">
          {/* Header */}
          <div className="bg-[#3F3F3F] text-white text-center font-bold text-sm px-3 py-2 uppercase tracking-wide">
            {status.issue_type}
          </div>

          {/* Message */}
          <div className="px-4 py-4 text-black text-sm text-center font-medium whitespace-pre-wrap">
            {status.message}
          </div>

          {/* Footer buttons */}
          <div className="flex items-center justify-around px-4 pb-4 pt-2 gap-2">
            {status.custom_action && (
              <button
                type="button"
                className="flex-1 h-[44px] bg-[#004FD6] text-white font-semibold text-sm shadow outline-none active:brightness-90 rounded-none border-0"
                onClick={handleCustomAction}
              >
                {status.custom_action.label}
              </button>
            )}
            <button
              type="button"
              className="flex-1 h-[44px] bg-[#3F3F3F] text-white font-semibold text-sm shadow outline-none active:brightness-90 rounded-none border-0"
              onClick={handleAcknowledge}
            >
              ACKNOWLEDGE
            </button>
            <button
              type="button"
              className="flex-1 h-[44px] bg-[#3F3F3F] text-white font-semibold text-sm shadow outline-none active:brightness-90 rounded-none border-0"
              onClick={() => onOpenChange(false)}
            >
              ESC
            </button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
