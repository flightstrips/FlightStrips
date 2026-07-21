import type { TacticalStrip } from "@/api/models";
import { useMyPosition, useWebSocketStore } from "@/store/store-hooks";
import { FONT, COLOR_BTN_BLUE } from "./shared";
import { TacticalActionCell, TacticalStripShell } from "./TacticalStripShell";

const CELL_BORDER_CLR = "#d9d9d9"; // light grey cell borders on memaid strip

interface Props {
  strip: TacticalStrip;
  width?: string | number;
}

export function TacticalMemaidStrip({ strip, width }: Props) {
  const myPosition = useMyPosition();
  const confirmTacticalStrip = useWebSocketStore(s => s.confirmTacticalStrip);

  const isOwner = strip.owner === myPosition;
  const label = strip.aircraft ? `${strip.label} (${strip.aircraft})` : strip.label;

  const canConfirm = !isOwner && !strip.confirmed;
  const confirmAction = (
    <TacticalActionCell
      borderColor={CELL_BORDER_CLR}
      color="white"
      clickable={canConfirm}
      onClick={canConfirm ? () => confirmTacticalStrip(strip.id) : undefined}
    >
      <span style={{ fontFamily: FONT, fontSize: "0.68vw", opacity: isOwner && !strip.confirmed ? 0.35 : 1 }}>
        {strip.confirmed ? "✓" : "⌛"}
      </span>
    </TacticalActionCell>
  );

  return (
    <TacticalStripShell
      strip={strip}
      width={width}
      backgroundColor={COLOR_BTN_BLUE}
      borderColor={CELL_BORDER_CLR}
      textColor="white"
      action={confirmAction}
      deleteHoverClass="hover:bg-primary/80"
    >
      {label}
    </TacticalStripShell>
  );
}
