import FlightPlanDialog from "@/components/FlightPlanDialog";
import { useAirport, useStrip } from "@/store/store-hooks";
import { Bay } from "@/api/models";

interface DepartureAwareFlightPlanDialogProps {
  callsign: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  allowClearanceFromPlannedBay?: boolean;
}

export function DepartureAwareFlightPlanDialog({
  callsign,
  open,
  onOpenChange,
  allowClearanceFromPlannedBay = false,
}: DepartureAwareFlightPlanDialogProps) {
  const strip = useStrip(callsign);
  const airport = useAirport();
  const isDeparture = strip?.origin === airport && strip?.destination !== airport;
  const isNotCleared = strip?.bay === Bay.NotCleared;
  const alreadyCleared = strip?.pdc_state === "CLEARED"
    || strip?.pdc_state === "CONFIRMED"
    || [Bay.Cleared, Bay.Push, Bay.Taxi, Bay.TaxiLwr, Bay.DeIce, Bay.Depart, Bay.Airborne, Bay.HiddenDep, Bay.DepHidden]
      .some((bay) => bay === strip?.bay);
  const clearanceMode = isDeparture && !alreadyCleared && (isNotCleared || allowClearanceFromPlannedBay);

  return (
    <FlightPlanDialog
      callsign={callsign}
      open={open}
      onOpenChange={onOpenChange}
      mode={clearanceMode ? "clearance" : "view"}
      pdcAction={clearanceMode ? "manual" : "default"}
    />
  );
}
