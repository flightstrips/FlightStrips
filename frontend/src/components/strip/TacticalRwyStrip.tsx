import type { TacticalStrip } from "@/api/models";
import { COLOR_BTN_ORANGE } from "./shared";
import { TacticalStripShell } from "./TacticalStripShell";

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
      backgroundColor={COLOR_BTN_ORANGE}
      borderColor={CELL_BORDER_CLR}
      textColor="white"
      deleteHoverClass="hover:bg-orange-600"
    >
      {label}
    </TacticalStripShell>
  );
}
