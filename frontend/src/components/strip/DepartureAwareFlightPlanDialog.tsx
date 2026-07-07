import FlightPlanDialog from "@/components/FlightPlanDialog";
import { useAirport, useStrip } from "@/store/store-hooks";
import { Bay } from "@/api/models";

interface DepartureAwareFlightPlanDialogProps {
  callsign: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function DepartureAwareFlightPlanDialog({
  callsign,
  open,
  onOpenChange,
}: DepartureAwareFlightPlanDialogProps) {
  const strip = useStrip(callsign);
  const airport = useAirport();
  const isDeparture = strip?.origin === airport && strip?.destination !== airport;
  const isNotCleared = strip?.bay === Bay.NotCleared;
  const clearanceMode = isDeparture && isNotCleared;

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
