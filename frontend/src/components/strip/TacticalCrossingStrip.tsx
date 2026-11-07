import type { TacticalStrip } from "@/api/models";
import { useMyPosition, useWebSocketStore } from "@/store/store-hooks";
import { FONT } from "./shared";
import hourglassIcon from "@/assets/hourglass.svg";
import { TacticalActionCell, TacticalStripShell } from "./TacticalStripShell";

const STRIP_BG        = "#fcc800"; // amber/gold crossing strip background
const CELL_BORDER_CLR = "#b39200"; // darker gold for cell borders / bottom border

interface Props {
  strip: TacticalStrip;
  width?: string | number;
}

export function TacticalCrossingStrip({ strip, width }: Props) {
  const myPosition = useMyPosition();
  const confirmTacticalStrip = useWebSocketStore(s => s.confirmTacticalStrip);

  const isOwner = strip.owner === myPosition;
  const label = strip.aircraft ? `${strip.label} (${strip.aircraft})` : strip.label;

  const canConfirm = !isOwner && !strip.confirmed;
  const confirmAction = (
    <TacticalActionCell
      color="black"
      clickable={canConfirm}
      onClick={canConfirm ? () => confirmTacticalStrip(strip.id) : undefined}
    >
      <span style={{ fontFamily: FONT, fontSize: "1.02vw", opacity: isOwner && !strip.confirmed ? 0.35 : 1 }}>
        {strip.confirmed ? "✓" : <img src={hourglassIcon} alt="Hourglass" draggable={false} style={{ height: "1.1vw", width: "auto", filter: "brightness(0)" }} />}
      </span>
    </TacticalActionCell>
  );

  return (
    <TacticalStripShell
      strip={strip}
      width={width}
      backgroundColor={STRIP_BG}
      borderColor={CELL_BORDER_CLR}
      textColor="black"
      action={confirmAction}
      deleteHoverClass=""
    >
      {label}
    </TacticalStripShell>
  );
}
