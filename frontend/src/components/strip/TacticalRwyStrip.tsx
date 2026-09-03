import type { TacticalStrip } from "@/api/models";
import { TacticalStripShell } from "./TacticalStripShell";

const STRIP_BG = "#dd6a12";
const CELL_BORDER_CLR = "#a04a00"; // dark burnt-orange cell borders on rwy strip

interface Props {
  strip: TacticalStrip;
  width?: string | number;
}

export function TacticalRwyStrip({ strip, width }: Props) {
  const label = strip.aircraft
    ? `${strip.type}${strip.label ? ` ${strip.label}` : ""} (${strip.aircraft})`
    : `${strip.type}${strip.label ? ` ${strip.label}` : ""}`;

  return (
    <TacticalStripShell
      strip={strip}
      width={width}
      backgroundColor={STRIP_BG}
      borderColor={CELL_BORDER_CLR}
      textColor="white"
      deleteHoverClass="hover:bg-orange-600"
    >
      {label}
    </TacticalStripShell>
  );
}
