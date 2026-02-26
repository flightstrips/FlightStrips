import type { StripProps } from "./types";
import { useSelectedCallsign, useSelectStrip } from "@/store/store-hooks";

const CELL_BORDER = "border-r border-[#85b4af]";
const TOP_H = 32; // 2/3 of 48px
const BOT_H = 16; // 1/3 of 48px

/**
 * ApnArrStrip — APN-TAXI-ARR strip used in TWY ARR and STAND bays (status="ARR").
 *
 * 48px strip with 2/3 (32px) top row / 1/3 (16px) bottom row vertical layout:
 *   [40px SI] | [120px callsign] | [80px actype↑ / reg↓] | [54px RWY] | [54px HS] | [80px stand]
 *
 * Background: yellow (#fff28e).
 */
export function ApnArrStrip({
  callsign,
  aircraftType,
  runway,
  taxiway,
  holdingPoint,
  stand,
  owner,
  nextControllers,
  previousControllers,
  myIdentifier,
  selectable,
}: StripProps) {
  const selectedCallsign = useSelectedCallsign();
  const selectStrip = useSelectStrip();
  const isSelected = selectable && selectedCallsign === callsign;

  const handleClick = selectable
    ? () => selectStrip(isSelected ? null : callsign)
    : undefined;

  const isAssumed = !!myIdentifier && owner === myIdentifier;
  const isTransferredAway =
    !!myIdentifier &&
    !!previousControllers?.includes(myIdentifier) &&
    !nextControllers?.includes(myIdentifier);

  let siBg = "#E082E7";
  if (isAssumed) siBg = "#F0F0F0";
  else if (isTransferredAway) siBg = "#DD6A12";

  return (
    <div
      className={`flex text-black select-none${isSelected ? " outline outline-2 outline-[#FF00F5]" : ""}${selectable ? " cursor-pointer" : ""}`}
      style={{
        height: 48,
        width: 428,
        backgroundColor: "#fff28e",
        borderLeft: "2px solid white",
        borderRight: "2px solid white",
        borderTop: "2px solid white",
        borderBottom: "1px solid white",
        boxShadow: "1px 0 0 0 #2F2F2F, 0 -1px 0 0 #2F2F2F",
      }}
      onClick={handleClick}
    >
      {/* SI / ownership — 8% of strip width */}
      <div
        className={`flex-shrink-0 flex items-center justify-center text-sm font-bold ${CELL_BORDER}`}
        style={{ width: 40, height: "100%", backgroundColor: siBg }}
      />

      {/* Callsign — 120px */}
      <div className={`flex-shrink-0 flex flex-col ${CELL_BORDER}`} style={{ width: 120, height: "100%" }}>
        <div className="flex items-center pl-2" style={{ height: TOP_H }}>
          <span className="font-bold text-xl truncate w-full">{callsign}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* A/C type / Registration — 80px */}
      <div className={`flex-shrink-0 flex flex-col ${CELL_BORDER}`} style={{ width: 80, height: "100%" }}>
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="text-xs font-semibold truncate px-1">{aircraftType}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* RWY — 54px */}
      <div className={`flex-shrink-0 flex flex-col overflow-hidden ${CELL_BORDER}`} style={{ width: 54, height: "100%" }}>
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="font-bold text-xl truncate">{runway}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* HS / Taxiway — 54px */}
      <div className={`flex-shrink-0 flex flex-col overflow-hidden ${CELL_BORDER}`} style={{ width: 54, height: "100%" }}>
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="font-bold text-xl truncate">{taxiway ?? holdingPoint}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* Stand — 80px */}
      <div className="flex-shrink-0 flex flex-col overflow-hidden" style={{ width: 80, height: "100%" }}>
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="font-bold text-xl truncate">{stand}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>
    </div>
  );
}
