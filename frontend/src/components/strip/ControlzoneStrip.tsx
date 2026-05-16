import { useMemo, useState } from "react";
import type { FrontendStrip } from "@/api/models";
import { useAirport, useMetar } from "@/store/store-hooks";
import { decodeMetar } from "@/lib/metarDecode";
import FlightPlanDialog from "@/components/FlightPlanDialog";
import {
  COLOR_ARR_YELLOW,
  COLOR_MANUAL_BLUE,
  FONT,
  getFlatStripBorderStyle,
  SELECTION_COLOR,
  useStripSelection,
} from "./shared";

const FULL_H = "4.72dvh";
const TOP_H = "3.15dvh";
const BOT_H = "1.57dvh";
const HALF_H = "2.08dvh";
const STRIP_W = "90%";
const CELL_BORDER = "var(--color-cell-border-arr)";

// Ratios taken from the 1080p controlzone.svg design: 34 | 120 | 80 | 52 | 60 | 76
const F_SI = 34;
const F_CALLSIGN = 120;
const F_SQUAWK = 80;
const F_META = 52;
const F_STATUS = 60;
const F_BLANK = 76;

// Typography derived from the 1080p SVG font sizes.
const CALLSIGN_FONT = "1.04vw"; // 20px
const SQUAWK_FONT = "0.83vw";   // 16px
const PRIMARY_FONT = "0.52vw";  // 10px
const META_VALUE_FONT = "0.47vw";
const META_LABEL_FONT = "0.39vw";
const EKCH_ELEVATION_FEET = 17;
const EKCH_AIRBORNE_THRESHOLD_AGL = 200;

interface Props {
  strip: FrontendStrip;
  selectable?: boolean;
}

function getControlzoneStatus(positionAltitude: number | undefined, airport: string): "GROUND" | "AIRBORNE" {
  if (airport !== "EKCH" || positionAltitude == null) {
    return "GROUND";
  }

  return positionAltitude > EKCH_ELEVATION_FEET + EKCH_AIRBORNE_THRESHOLD_AGL ? "AIRBORNE" : "GROUND";
}

export function ControlzoneStrip({ strip, selectable }: Props) {
  const airport = useAirport();
  const metar = useMetar();
  const decodedMetar = useMemo(() => decodeMetar(metar), [metar]);
  const [fplOpen, setFplOpen] = useState(false);
  const { isSelected, handleClick } = useStripSelection(strip.callsign, selectable);

  const squawk = strip.assigned_squawk?.trim() || strip.squawk?.trim() || "7000";
  const qnh = decodedMetar.qnh != null ? String(decodedMetar.qnh) : "XXXX";
  const personsOnBoard = strip.persons_on_board && strip.persons_on_board > 0
    ? String(strip.persons_on_board)
    : "";
  const remarks = strip.remarks?.trim() ? `:${strip.remarks.trim()}` : "";
  const language = strip.language?.trim() || "";
  const fplType = strip.fpl_type?.trim() || "";
  const statusLabel = getControlzoneStatus(strip.position_altitude, airport);

  return (
    <>
    <div
      className="select-none"
      style={{
        height: FULL_H,
        width: STRIP_W,
        backgroundColor: COLOR_ARR_YELLOW,
        ...getFlatStripBorderStyle({}, CELL_BORDER),
      }}
      onClick={handleClick}
    >
      <div className="flex h-full overflow-hidden text-black">
        <div
          className="flex-shrink-0 border-r-2"
          style={{ flex: `${F_SI} 0 0%`, backgroundColor: "#F0F0F0", borderRightColor: CELL_BORDER }}
        />

        <div
          className="flex flex-col overflow-hidden border-r-2"
          style={{ flex: `${F_CALLSIGN} 0 0%`, borderRightColor: CELL_BORDER }}
        >
          <div className="flex items-center px-[0.42vw] overflow-hidden" style={{ height: TOP_H, backgroundColor: isSelected ? SELECTION_COLOR : undefined }}>
            <span
              className="truncate w-full"
              style={{ fontFamily: FONT, fontSize: CALLSIGN_FONT, fontWeight: 700, color: COLOR_MANUAL_BLUE }}
            >
              {strip.callsign}
            </span>
          </div>
          <div className="flex items-center px-[0.42vw] overflow-hidden" style={{ height: BOT_H }}>
            <span
              className="truncate w-full"
              style={{ fontFamily: FONT, fontSize: PRIMARY_FONT, textTransform: "uppercase" }}
            >
              {remarks}
            </span>
          </div>
        </div>

        <div
          className="flex flex-col overflow-hidden border-r-2"
          style={{ flex: `${F_SQUAWK} 0 0%`, borderRightColor: CELL_BORDER }}
        >
          <div style={{ height: HALF_H }} />
          <div className="flex items-center justify-center overflow-hidden" style={{ height: HALF_H }}>
            <span
              className="truncate px-[0.21vw]"
              style={{ fontFamily: FONT, fontSize: SQUAWK_FONT, fontWeight: 600, color: COLOR_MANUAL_BLUE }}
            >
              {squawk}
            </span>
          </div>
        </div>

        <div
          className="flex flex-col overflow-hidden border-r-2"
          style={{ flex: `${F_META} 0 0%`, borderRightColor: CELL_BORDER }}
        >
          <div className="flex items-center gap-[0.16vw] px-[0.21vw]" style={{ height: HALF_H }}>
            <span style={{ fontFamily: FONT, fontSize: META_LABEL_FONT, lineHeight: 1 }}>QNH:</span>
            <span style={{ fontFamily: FONT, fontSize: META_VALUE_FONT, fontWeight: 700, lineHeight: 1 }}>{qnh}</span>
          </div>
          <div className="flex items-center gap-[0.16vw] px-[0.21vw]" style={{ height: HALF_H }}>
            <span style={{ fontFamily: FONT, fontSize: META_LABEL_FONT, lineHeight: 1 }}>POB:</span>
            <span style={{ fontFamily: FONT, fontSize: META_VALUE_FONT, fontWeight: 700, lineHeight: 1 }}>{personsOnBoard}</span>
          </div>
        </div>

        <div
          className="flex flex-col overflow-hidden border-r-2"
          style={{ flex: `${F_STATUS} 0 0%`, borderRightColor: CELL_BORDER }}
        >
          <div className="flex items-center justify-center overflow-hidden" style={{ height: HALF_H }}>
            <span style={{ fontFamily: FONT, fontSize: PRIMARY_FONT, fontWeight: 700 }}>
              {statusLabel}
            </span>
          </div>
          <div className="flex border-t-2" style={{ height: HALF_H, borderTopColor: CELL_BORDER }}>
            <div className="flex items-center justify-center overflow-hidden border-r-2" style={{ flex: "1 0 0%", borderRightColor: CELL_BORDER }}>
              <span style={{ fontFamily: FONT, fontSize: PRIMARY_FONT, fontWeight: 700, textTransform: "uppercase" }}>
                {language}
              </span>
            </div>
            <div className="flex items-center justify-center overflow-hidden" style={{ flex: "1 0 0%" }}>
              <span style={{ fontFamily: FONT, fontSize: PRIMARY_FONT, fontWeight: 700, textTransform: "uppercase" }}>
                {fplType}
              </span>
            </div>
          </div>
        </div>

        <div
          className="overflow-hidden cursor-pointer hover:brightness-95"
          style={{ flex: `${F_BLANK} 0 0%` }}
          onClick={(e) => { e.stopPropagation(); setFplOpen(true); }}
        />
      </div>
    </div>
    <FlightPlanDialog callsign={strip.callsign} open={fplOpen} onOpenChange={setFplOpen} mode="view" />
    </>
  );
}
