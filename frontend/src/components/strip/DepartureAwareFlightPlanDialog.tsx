import FlightPlanDialog from "@/components/FlightPlanDialog";
import { useAirport, useStrip } from "@/store/store-hooks";

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

  return (
    <FlightPlanDialog
      callsign={callsign}
      open={open}
      onOpenChange={onOpenChange}
      mode={isDeparture ? "clearance" : "view"}
      pdcAction={isDeparture ? "manual" : "default"}
    />
  );
}
