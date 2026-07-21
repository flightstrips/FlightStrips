import type { TacticalStrip } from "@/api/models";
import { TacticalStripShell } from "./TacticalStripShell";

const STRIP_BG        = "#fcc800"; // amber/gold crossing strip background
const CELL_BORDER_CLR = "#b39200"; // darker gold for cell borders / bottom border

interface Props {
  strip: TacticalStrip;
  width?: string | number;
}

export function TacticalCrossingStrip({ strip, width }: Props) {
  const label = strip.aircraft ? `${strip.label} (${strip.aircraft})` : strip.label;

  return (
    <TacticalStripShell
      strip={strip}
      width={width}
      backgroundColor={STRIP_BG}
      borderColor={CELL_BORDER_CLR}
      textColor="black"
      deleteHoverClass="hover:bg-yellow-400"
    >
      {label}
    </TacticalStripShell>
  );
}
