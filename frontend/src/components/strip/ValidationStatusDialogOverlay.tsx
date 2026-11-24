import { ValidationStatusDialog } from "@/components/strip/ValidationStatusDialog";
import {
  useCloseValidationDialog,
  useMyPosition,
  useValidationDialogCallsign,
  useWebSocketStore,
} from "@/store/store-hooks";
import { isValidationActiveForPosition } from "@/lib/validation-status";

export function ValidationStatusDialogOverlay() {
  const callsign = useValidationDialogCallsign();
  const closeValidationDialog = useCloseValidationDialog();
  const myPosition = useMyPosition();
  const strip = useWebSocketStore((state) => state.strips.find((strip) => strip.callsign === callsign));
  const status = strip?.validation_status;
  const liveStatus = isValidationActiveForPosition(status, myPosition, strip?.owner) ? status : undefined;

  if (!callsign || !liveStatus) {
    return null;
  }

  return (
    <ValidationStatusDialog
      callsign={callsign}
      status={liveStatus}
      open={true}
      onOpenChange={(open) => {
        if (!open) {
          closeValidationDialog();
        }
      }}
    />
  );
}
