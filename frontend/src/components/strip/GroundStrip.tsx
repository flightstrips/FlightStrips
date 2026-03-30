import { getAircraftTypeWithWtc } from "@/lib/utils";
import { getStripBg } from "./types";
import type { StripProps } from "./types";
import { useStripSelection, getCellBorderColor, getFlatStripBorderStyle, SELECTION_COLOR, COLOR_TYPE_HEAVY, getStripOwnership, useStripBg } from "./shared";
import { useStripTransfers, useWebSocketStore } from "@/store/store-hooks";
import { SIBox } from "./SIBox";

const TOP_H = "2.96vh"; // 2/3 of 48px
const BOT_H = "1.48vh"; // 1/3 of 48px

// -----------------------------------------------------------------------------
// GroundStrip — shown after clearance is issued (status="CLROK") — TWY DEP
//
// 48px strip with 2/3 (32px) top row / 1/3 (16px) bottom row vertical layout:
//   [40px SI] | [120px callsign] | [80px actype/reg] | [80px stand] | [80px clearance limit] | [80px RWY] | [27px box]
//
// Background: cyan (var(--color-strip-dep-bg)).
// -----------------------------------------------------------------------------

export function GroundStrip({
  callsign,
  bay,
  pdcStatus,
  aircraftType,
  aircraftCategory,
  stand,
  taxiway,
  holdingPoint,
  runway,
  arrival,
  owner,
  nextControllers,
  previousControllers,
  myPosition,
  selectable,
  marked = false,
}: StripProps) {
  const { isSelected, handleClick } = useStripSelection(callsign, selectable);
  const cellBorderColor = getCellBorderColor(marked);
  const stripTransfers = useStripTransfers();
  const isTagRequest = !!stripTransfers[callsign]?.isTagRequest;
  const openStripContextMenu = useWebSocketStore(s => s.openStripContextMenu);

  const { isUnconcerned } = getStripOwnership(myPosition, owner, nextControllers, previousControllers);
  const { bg, textWhite } = useStripBg(runway, getStripBg(pdcStatus, arrival, bay), isTagRequest, isUnconcerned, pdcStatus, bay);

  return (
    <div
      className={`flex ${textWhite ? "text-white" : "text-black"} select-none`}
      style={{
        height: "4.44vh",
        width: "25vw",
        backgroundColor: bg,
        ...getFlatStripBorderStyle({ borderBottom: "1px solid white" }),
      }}
    >
      <SIBox
        callsign={callsign}
        owner={owner}
        nextControllers={nextControllers}
        previousControllers={previousControllers}
        myPosition={myPosition}
        marked={marked}
        transferringTo={stripTransfers[callsign]?.to ?? ""}
        isTagRequest={isTagRequest}
      />

      {/* Callsign — 120px */}
      <div className="flex-shrink-0 flex flex-col border-r-2 cursor-pointer" style={{ width: "6.25vw", height: "100%", borderRightColor: cellBorderColor }}
        onClick={handleClick}
        onContextMenu={(e) => { e.preventDefault(); openStripContextMenu(callsign, { x: e.clientX, y: e.clientY }); }}
      >
        <div className="flex items-center pl-[0.42vw]" style={{ height: TOP_H, backgroundColor: isSelected ? SELECTION_COLOR : undefined }}>
          <span className="truncate w-full" style={{ fontWeight: "bold", fontSize: "1.04vw" }}>{callsign}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* A/C type — 80px split (bottom reserved for registration) */}
      <div className="flex-shrink-0 flex flex-col border-r-2" style={{ width: "4.17vw", height: "100%", borderRightColor: cellBorderColor }}>
        <div className="flex items-center justify-center border-b-2" style={{ height: TOP_H, borderBottomColor: cellBorderColor }}>
          <span className="truncate px-[0.21vw]" style={{ fontWeight: 600, fontSize: "0.63vw", color: aircraftCategory === "H" ? COLOR_TYPE_HEAVY : undefined }}>{getAircraftTypeWithWtc(aircraftType, aircraftCategory)}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* Stand — 80px */}
      <div className="flex-shrink-0 flex flex-col border-r-2" style={{ width: "4.17vw", height: "100%", borderRightColor: cellBorderColor }}>
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="truncate" style={{ fontWeight: "bold", fontSize: "1.04vw" }}>{stand}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* Clearance limit — 80px */}
      <div className="flex-shrink-0 flex flex-col border-r-2" style={{ width: "4.17vw", height: "100%", borderRightColor: cellBorderColor }}>
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="truncate" style={{ fontWeight: "bold", fontSize: "1.04vw" }}>{taxiway ?? holdingPoint}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>

      {/* RWY — 80px */}
      <div className="flex-shrink-0 flex flex-col overflow-hidden" style={{ width: "4.17vw", height: "100%" }}>
        <div className="flex items-center justify-center" style={{ height: TOP_H }}>
          <span className="truncate" style={{ fontWeight: "bold", fontSize: "1.04vw" }}>{runway}</span>
        </div>
        <div style={{ height: BOT_H }} />
      </div>
    </div>
  );
}
