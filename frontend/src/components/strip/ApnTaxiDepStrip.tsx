import type { StripProps } from "./types";
import { useSelectedCallsign, useSelectStrip } from "@/store/store-hooks";
import { getSimpleAircraftType } from "@/lib/utils";
import { SIBox } from "./shared";

const CELL_BORDER = "border-r border-[#85b4af]";
const RUBIK = "'Rubik', sans-serif";
const TOP_H  = "2.96vh";  // 2/3 of 4.44vh
const BOT_H  = "1.48vh";  // 1/3 of 4.44vh
const HALF_H = "calc(2.22vh - 3px)";  // 1/2 of inner content height (4.44vh - 2px border - 1px padding each side)

// Flex-grow proportions (flex-basis: 0 so space is shared proportionally)
const F_CALLSIGN = 25;
const F_TYPE     = 25 * (2 / 3);            // ~16.67
const F_STAND    = 25 * (2 / 3);            // ~16.67
const F_HP       = 25 * (2 / 3) * (2 / 3); // ~11.11
const F_RWY      = 25 * (2 / 3);            // ~16.67

/**
 * ApnTaxiDepStrip — APN-TAXI-DEP strip (status="TAXI-DEP").
 *
 * Width: 90% of bay. Cells use flex proportions:
 *   SI 8 | Callsign 25 | Type+Reg 25*(2/3) | Stand 25*(2/3) | HP 25*(2/3)*(2/3) | RWY 25*(2/3)
 *
 * Background: cyan (#bef5ef).
 */
export function ApnTaxiDepStrip({
  callsign,
  aircraftType,
  stand,
  holdingPoint,
  runway,
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

  return (
    <div
      className={`select-none${isSelected ? " outline outline-2 outline-[#FF00F5]" : ""}${selectable ? " cursor-pointer" : ""}`}
      style={{
        height: "4.44vh",
        width: "90%",
        backgroundColor: "#85b4af",
        padding: "1px",
        borderLeft: "2px solid white",
        borderRight: "2px solid white",
        borderTop: "2px solid white",
        borderBottom: "2px solid white",
        boxShadow: "1px 0 0 0 #2F2F2F, 0 -1px 0 0 #2F2F2F",
      }}
      onClick={handleClick}
    >
      <div className="flex text-black" style={{ height: "100%", overflow: "hidden", backgroundColor: "#bef5ef" }}>

        {/* SI / ownership — 8% */}
        <SIBox
          owner={owner}
          nextControllers={nextControllers}
          previousControllers={previousControllers}
          myIdentifier={myIdentifier}
        />

        {/* Callsign — 25%, Rubik medium 20, top 2/3 highlighted when selected */}
        <div
          className={`flex flex-col overflow-hidden ${CELL_BORDER}`}
          style={{ flex: `${F_CALLSIGN} 0 0%`, height: "100%", minWidth: 0 }}
        >
          <div className="flex items-center pl-2" style={{ height: TOP_H, backgroundColor: isSelected ? "#FF00F5" : undefined }}>
            <span className="truncate w-full" style={{ fontFamily: RUBIK, fontWeight: 500, fontSize: 20 }}>
              {callsign}
            </span>
          </div>
          <div style={{ height: BOT_H }} />
        </div>

        {/* A/C type / Registration — 25%*(2/3), stacked in top 2/3 */}
        <div
          className={`flex flex-col items-center justify-center overflow-hidden ${CELL_BORDER}`}
          style={{ flex: `${F_TYPE} 0 0%`, height: "100%", paddingBottom: BOT_H, minWidth: 0 }}
        >
          <span className="truncate px-1 leading-tight w-full text-center" style={{ fontFamily: RUBIK, fontWeight: 400, fontSize: 10 }}>
            {getSimpleAircraftType(aircraftType)}
          </span>
          <span className="truncate px-1 leading-tight w-full text-center" style={{ fontFamily: RUBIK, fontWeight: 400, fontSize: 10 }}>
            OYFSR
          </span>
        </div>

        {/* Stand — 25%*(2/3), value in top 2/3 */}
        <div
          className={`flex items-center justify-center overflow-hidden ${CELL_BORDER}`}
          style={{ flex: `${F_STAND} 0 0%`, height: "100%", paddingBottom: BOT_H, minWidth: 0 }}
        >
          <span style={{ fontFamily: RUBIK, fontWeight: 600, fontSize: 20 }}>{stand}</span>
        </div>

        {/* Holding Point — 25%*(2/3)*(2/3)
              top 1/2: HP value separated by a border line
              bottom 1/2: faint "HP" label */}
        <div
          className={`flex flex-col overflow-hidden ${CELL_BORDER}`}
          style={{ flex: `${F_HP} 0 0%`, height: "100%", minWidth: 0 }}
        >
          <div className="flex items-center justify-center border-b border-[#85b4af]" style={{ height: HALF_H }}>
            <span style={{ fontFamily: RUBIK, fontWeight: 600, fontSize: 11 }}>{holdingPoint}</span>
          </div>
          <div className="flex items-center justify-center" style={{ height: HALF_H }}>
            <span style={{ fontFamily: RUBIK, fontWeight: 700, fontSize: 14, opacity: 0.2 }}>HP</span>
          </div>
        </div>

        {/* Runway — 25%*(2/3)
              top 1/2: left 2/3 = runway text, right 1/3 = empty box
              bottom 1/2: empty */}
        <div
          className="flex flex-col overflow-hidden"
          style={{ flex: `${F_RWY} 0 0%`, height: "100%", minWidth: 0 }}
        >
          <div className="flex" style={{ height: HALF_H }}>
            <div className="flex items-center justify-center" style={{ flex: "2 0 0%", height: "100%" }}>
              <span style={{ fontFamily: RUBIK, fontWeight: 600, fontSize: 14 }}>{runway}</span>
            </div>
            <div style={{ flexShrink: 0, width: HALF_H, height: "100%", borderLeft: "1px solid #85b4af", borderBottom: "1px solid #85b4af" }} />
          </div>
          <div style={{ height: HALF_H }} />
        </div>

      </div>
    </div>
  );
}
