import { useStripTransfers } from "@/store/store-hooks";
import type { StripProps } from "./types";
import { useStripSelection, getCellBorderColor, getFlatStripBorderStyle, SELECTION_COLOR, FONT, COLOR_ARR_YELLOW } from "./shared";
import { SIBox } from "./SIBox";

/** Gold cell borders — matches the yellow arrival strip design. */
const CELL_BORDER = "#FFD100";

/** Subdued text color for the holding point secondary row. */
const COLOR_HP_TEXT = "#5F5F5F";

// Heights — 48px total fixed (intentional — ATC arrival strip spec), 2/3 top / 1/3 bottom
const TOP_H = 32;
const BOT_H = 16;

// Fixed cell widths (SI fills the remaining 40px via SIBox flex-grow)
const W_CALLSIGN = 120;
const W_TYPE     = 80;
const W_TAXIWAY  = 80;
const W_RWY      = 54;
const W_STAND    = 80;

export const TOTAL_W = 40 + W_CALLSIGN + W_TYPE + W_TAXIWAY + W_RWY + W_STAND;

/**
 * FinalArrStrip — strip for FINAL, RWY-ARR, and TWY-ARR bays (status="FINAL-ARR").
 *
 * 48px strip with 2/3 (32px) top row / 1/3 (16px) bottom row vertical layout:
 *   [40px SI] | [120px callsign] | [80px type↑ / squawk↓] |
 *   [80px taxiway↑] | [54px runway↑ / HP↓] | [80px stand]
 *
 * Background: yellow (#fff28e). Cell borders: gold (#FFD100).
 */
export function FinalArrStrip({
  callsign,
  aircraftType,
  squawk,
  runway,
  taxiway,
  holdingPoint,
  stand,
  owner,
  nextControllers,
  previousControllers,
  myPosition,
  selectable,
  marked = false,
}: StripProps) {
  const { isSelected, handleClick } = useStripSelection(callsign, selectable);
  const cellBorderColor = getCellBorderColor(marked, CELL_BORDER);
  const stripTransfers = useStripTransfers();

  return (
    <div
      className={`flex text-black select-none${selectable ? " cursor-pointer" : ""}`}
      style={{
        height: 48, // 48px fixed — intentional ATC arrival strip height
        width: TOTAL_W,
        backgroundColor: COLOR_ARR_YELLOW,
        ...getFlatStripBorderStyle(),
      }}
      onClick={handleClick}
    >
      {/* SI / ownership — 40px (fills via SIBox flex-grow) */}
      <SIBox
        callsign={callsign}
        owner={owner}
        nextControllers={nextControllers}
        previousControllers={previousControllers}
        myPosition={myPosition}
        marked={marked}
        transferringTo={stripTransfers[callsign] ?? ""}
        baseBorderColor={CELL_BORDER}
      />

      {/* Callsign — 120px; top 2/3 = callsign (2/3 cell height per spec) */}
      <div
        className="flex-shrink-0 flex flex-col border-r-2"
        style={{ width: W_CALLSIGN, height: "100%", borderRightColor: cellBorderColor }}
      >
        <div
          className="flex items-center pl-2 overflow-hidden"
          style={{ height: TOP_H, backgroundColor: isSelected ? SELECTION_COLOR : undefined }}
        >
          <span className="truncate w-full" style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 20 }}>
            {callsign}
          </span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* Type / Squawk — 80px */}
      <div
        className="flex-shrink-0 flex flex-col border-r-2"
        style={{ width: W_TYPE, height: "100%", borderRightColor: cellBorderColor }}
      >
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="truncate px-1" style={{ fontFamily: FONT, fontWeight: 600, fontSize: 12 }}>
            {aircraftType}
          </span>
        </div>
        <div className="flex items-center justify-center" style={{ height: BOT_H }}>
          <span className="truncate px-1" style={{ fontFamily: FONT, fontSize: 12 }}>
            {squawk}
          </span>
        </div>
      </div>

      {/* Taxiway — 80px; bottom row reserved for ETA when available */}
      <div
        className="flex-shrink-0 flex flex-col border-r-2"
        style={{ width: W_TAXIWAY, height: "100%", borderRightColor: cellBorderColor }}
      >
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="truncate px-1" style={{ fontFamily: FONT, fontWeight: 600, fontSize: 16 }}>
            {stand}
          </span>
        </div>
        <div className="flex items-center justify-center" style={{ height: BOT_H }}>
          <span className="truncate px-1" style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 14 }}>
            {taxiway}
          </span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* Runway / Holding Point — 54px */}
      <div
        className="flex-shrink-0 flex flex-col border-r-2"
        style={{ width: W_RWY, height: "100%", borderRightColor: cellBorderColor }}
      >
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="truncate" style={{ fontFamily: FONT, fontWeight: "bold", fontSize: 18 }}>
            {runway}
          </span>
        </div>
        <div className="flex items-center justify-center" style={{ height: BOT_H }}>
          <span style={{ fontFamily: FONT, fontSize: 12, color: COLOR_HP_TEXT }}>
            {holdingPoint}
          </span>
        </div>
      </div>

      {/* Stand — 80px */}
      <div
        className="flex-shrink-0 flex flex-col overflow-hidden"
        style={{ width: W_STAND, height: "100%" }}
      >
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="truncate px-1" style={{ fontFamily: FONT, fontSize: 14 }}>
            NONE
          </span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>
    </div>
  );
}
