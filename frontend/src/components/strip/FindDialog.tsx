import { Bay } from "@/api/models";
import { StripListPopup } from "@/components/StripListPopup";
import { vfrSortModes } from "@/lib/stripSortModes";
import { useMyPosition, useStrips, useWebSocketStore } from "@/store/store-hooks";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function FindDialog({ open, onOpenChange }: Props) {
  const strips = useStrips();
  const myPosition = useMyPosition();
  const moveToControlzone = useWebSocketStore((state) => state.moveToControlzone);

  if (!open) return null;

  return (
    <StripListPopup
      title="FIND VFR"
      strips={strips.filter((strip) => strip.flight_rules === "V" && strip.has_fp !== false)}
      sortModes={vfrSortModes}
      onRowClick={(strip) => {
        if (strip.bay !== Bay.Controlzone) moveToControlzone(strip.callsign);
        onOpenChange(false);
      }}
      onDismiss={() => onOpenChange(false)}
      myPosition={myPosition}
    />
  );
}
