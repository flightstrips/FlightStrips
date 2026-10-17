import { useCallsign, useControllers, useMyPosition, useSelectedCallsign, useStrip, useWebSocketStore } from "@/store/store-hooks";
import { findStationController, type ControllerStation } from "@/lib/controllerStations";

interface TransferControllerButtonProps {
  label: string;
  station: ControllerStation;
  className: string;
}

export function TransferControllerButton({ label, station, className }: TransferControllerButtonProps) {
  const selectedCallsign = useSelectedCallsign();
  const myPosition = useMyPosition();
  const myCallsign = useCallsign();
  const strip = useStrip(selectedCallsign ?? "");
  const controllers = useControllers();
  const selectNextController = useWebSocketStore((state) => state.selectNextController);
  const target = findStationController(station, controllers, myPosition ?? "", myCallsign ?? "");
  const disabled = !selectedCallsign || strip?.owner !== myPosition || !target;

  return (
    <button
      type="button"
      disabled={disabled}
      className={className}
      onClick={() => {
        if (selectedCallsign && target && !disabled) {
          selectNextController(selectedCallsign, label);
        }
      }}
    >
      {label}
    </button>
  );
}
