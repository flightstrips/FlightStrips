import { Bay } from "@/api/models";
import { StripListPopup } from "@/components/StripListPopup.tsx";
import { plannedDepartureSortModes } from "@/lib/stripSortModes";
import { useAirport, useMyPosition, useStrips, useWebSocketStore } from "@/store/store-hooks";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function PlannedDialog({ open, onOpenChange }: Props) {
  const strips = useStrips();
  const airport = useAirport();
  const myPosition = useMyPosition();
  const move = useWebSocketStore((state) => state.move);

  if (!open) return null;

  return (
    <StripListPopup
      title="PLANNED DEP"
      rowHalfStripVariant="LOCKED-DEP"
      strips={strips.filter((strip) => strip.origin === airport)}
      sortModes={plannedDepartureSortModes}
      onRowClick={(strip) => {
        if (strip.bay !== Bay.NotCleared) move(strip.callsign, Bay.NotCleared);
        onOpenChange(false);
      }}
      onDismiss={() => onOpenChange(false)}
      myPosition={myPosition}
    />
  );
}
