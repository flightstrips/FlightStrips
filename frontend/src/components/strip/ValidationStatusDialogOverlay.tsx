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
  const status = useWebSocketStore((state) =>
    callsign
      ? state.strips.find((strip) => strip.callsign === callsign)?.validation_status
      : undefined,
  );

  const liveStatus = status && isValidationActiveForPosition(status, myPosition) ? status : undefined;

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
