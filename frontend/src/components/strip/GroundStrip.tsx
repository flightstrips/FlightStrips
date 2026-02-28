import { getStripBg } from "./types";
import type { StripProps } from "./types";
import { useStripSelection, getCellBorderColor, getFlatStripBorderStyle, SELECTION_COLOR } from "./shared";

const TOP_H = 32; // 2/3 of 48px
const BOT_H = 16; // 1/3 of 48px

// -----------------------------------------------------------------------------
// GroundStrip — shown after clearance is issued (status="CLROK") — TWY DEP
//
// 48px strip with 2/3 (32px) top row / 1/3 (16px) bottom row vertical layout:
//   [40px SI] | [120px callsign] | [80px actype/reg] | [80px stand] | [80px clearance limit] | [80px RWY] | [27px box]
//
// Background: cyan (#bef5ef).
// -----------------------------------------------------------------------------

export function GroundStrip({
  callsign,
  pdcStatus,
  aircraftType,
  stand,
  taxiway,
  holdingPoint,
  runway,
  arrival,
  owner,
  nextControllers,
  previousControllers,
  myIdentifier,
  selectable,
}: StripProps) {
  const { isSelected, handleClick } = useStripSelection(callsign, selectable);
  const cellBorderColor = getCellBorderColor(false);

  const isAssumed = !!myIdentifier && owner === myIdentifier;
  const isTransferredAway =
    !!myIdentifier &&
    !!previousControllers?.includes(myIdentifier) &&
    !nextControllers?.includes(myIdentifier);

  let siBg = "#E082E7";
  if (isAssumed) siBg = "#F0F0F0";
  else if (isTransferredAway) siBg = "#DD6A12";

  const nextLabel =
    isAssumed && nextControllers?.[0] ? nextControllers[0].slice(0, 2) : "";

  return (
    <div
      className={`flex text-black select-none${selectable ? " cursor-pointer" : ""}`}
      style={{
        height: 48,
        width: 480,
        backgroundColor: getStripBg(pdcStatus, arrival),
        ...getFlatStripBorderStyle({ borderBottom: "1px solid white" }),
      }}
      onClick={handleClick}
    >
      {/* SI / ownership — 40px */}
      <div
        className="flex-shrink-0 flex items-center justify-center text-sm font-bold border-r-2"
        style={{ width: 40, height: "100%", backgroundColor: siBg, borderRightColor: cellBorderColor }}
      >
        {nextLabel}
      </div>

      {/* Callsign — 120px */}
      <div className="flex-shrink-0 flex flex-col border-r-2" style={{ width: 120, height: "100%", borderRightColor: cellBorderColor }}>
        <div className="flex items-center pl-2" style={{ height: TOP_H, backgroundColor: isSelected ? SELECTION_COLOR : undefined }}>
          <span className="font-bold text-xl truncate w-full">{callsign}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* A/C type — 80px split (bottom reserved for registration) */}
      <div className="flex-shrink-0 flex flex-col border-r-2" style={{ width: 80, height: "100%", borderRightColor: cellBorderColor }}>
        <div className="flex items-center justify-center border-b-2" style={{ height: TOP_H, borderBottomColor: cellBorderColor }}>
          <span className="text-xs font-semibold truncate px-1">{aircraftType}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* Stand — 80px */}
      <div className="flex-shrink-0 flex flex-col border-r-2" style={{ width: 80, height: "100%", borderRightColor: cellBorderColor }}>
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="font-bold text-xl truncate">{stand}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* Clearance limit — 80px */}
      <div className="flex-shrink-0 flex flex-col border-r-2" style={{ width: 80, height: "100%", borderRightColor: cellBorderColor }}>
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="font-bold text-xl truncate">{taxiway ?? holdingPoint}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* RWY — 80px */}
      <div className="flex-shrink-0 flex flex-col overflow-hidden" style={{ width: 80, height: "100%" }}>
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="font-bold text-xl truncate">{runway}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>
    </div>
  );
}
