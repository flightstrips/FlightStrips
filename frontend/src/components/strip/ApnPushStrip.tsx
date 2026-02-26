import type { StripProps } from "./types";
import { useSelectedCallsign, useSelectStrip } from "@/store/store-hooks";

const CELL_BORDER = "border-r border-[#85b4af]";
const RUBIK = "'Rubik', sans-serif";
const HALF_H = "2.22vh"; // half of 4.44vh for TSAT/CTOT split

// Flex-grow proportions (flex-basis: 0 so space is shared proportionally)
const F_SI       = 8;
const F_CALLSIGN = 25;
const F_TYPE     = 25 * (2 / 3);          // ~16.67
const F_STAND    = 25 * (2 / 3);          // ~16.67
const F_TSAT     = 25 * (2 / 3);          // ~16.67
const F_RWY      = 25 * (2 / 3) * (2 / 3); // ~11.11

function SIBox({ owner, nextControllers, previousControllers, myIdentifier }: {
  owner?: string;
  nextControllers?: string[];
  previousControllers?: string[];
  myIdentifier?: string;
}) {
  const isAssumed = !!myIdentifier && owner === myIdentifier;
  const isTransferredAway =
    !!myIdentifier &&
    !!previousControllers?.includes(myIdentifier) &&
    !nextControllers?.includes(myIdentifier);

  let bgColor = "#E082E7";
  if (isAssumed) bgColor = "#F0F0F0";
  else if (isTransferredAway) bgColor = "#DD6A12";

  const nextLabel =
    isAssumed && nextControllers?.[0] ? nextControllers[0].slice(0, 2) : "";

  return (
    <div
      className={`flex items-center justify-center text-sm font-bold ${CELL_BORDER}`}
      style={{ flex: `${F_SI} 0 0%`, height: "100%", backgroundColor: bgColor, minWidth: 0 }}
    >
      {nextLabel}
    </div>
  );
}

/**
 * ApnPushStrip — APNPUSH strip for STARTUP, PUSH BACK and DE-ICE bays (status="PUSH").
 *
 * Width: 90% of bay. Cells use flex proportions:
 *   SI 8 | Callsign 25 | Type+Reg 25*(2/3) | Stand 25*(2/3) | TSAT/CTOT 25*(2/3) | RWY 25*(2/3)*(2/3)
 *
 * Background: cyan (#bef5ef).
 */
export function ApnPushStrip({
  callsign,
  aircraftType,
  stand,
  tsat,
  ctot,
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
      className={`flex text-black select-none${isSelected ? " outline outline-2 outline-[#FF00F5]" : ""}${selectable ? " cursor-pointer" : ""}`}
      style={{
        height: "4.44vh",
        width: "90%",
        backgroundColor: "#bef5ef",
        borderLeft: "2px solid white",
        borderRight: "2px solid white",
        borderTop: "2px solid white",
        borderBottom: "1px solid white",
        boxShadow: "1px 0 0 0 #2F2F2F, 0 -1px 0 0 #2F2F2F",
        overflow: "hidden",
      }}
      onClick={handleClick}
    >
      {/* SI / ownership — 8% */}
      <SIBox
        owner={owner}
        nextControllers={nextControllers}
        previousControllers={previousControllers}
        myIdentifier={myIdentifier}
      />

      {/* Callsign — 25%, Rubik medium 20 */}
      <div
        className={`flex items-center pl-2 overflow-hidden ${CELL_BORDER}`}
        style={{ flex: `${F_CALLSIGN} 0 0%`, height: "100%", paddingBottom: "1.48vh", minWidth: 0 }}
      >
        <span className="truncate w-full" style={{ fontFamily: RUBIK, fontWeight: 500, fontSize: 20 }}>
          {callsign}
        </span>
      </div>

      {/* A/C type / Registration — 25%*(2/3), Rubik regular 10, stacked top 2/3 no divider */}
      <div
        className={`flex flex-col items-center justify-center overflow-hidden ${CELL_BORDER}`}
        style={{ flex: `${F_TYPE} 0 0%`, height: "100%", paddingBottom: "1.48vh", minWidth: 0 }}
      >
        <span className="truncate px-1 leading-tight w-full text-center" style={{ fontFamily: RUBIK, fontWeight: 400, fontSize: 10 }}>{aircraftType?.split("/")[0]}</span>
        <span className="truncate px-1 leading-tight w-full text-center" style={{ fontFamily: RUBIK, fontWeight: 400, fontSize: 10 }}>OYFSR</span>
      </div>

      {/* Stand — 25%*(2/3), Rubik semibold 20 */}
      <div
        className={`flex items-center justify-center overflow-hidden ${CELL_BORDER}`}
        style={{ flex: `${F_STAND} 0 0%`, height: "100%", paddingBottom: "1.48vh", minWidth: 0 }}
      >
        <span style={{ fontFamily: RUBIK, fontWeight: 600, fontSize: 20 }}>{stand}</span>
      </div>

      {/* TSAT / CTOT — 25%*(2/3), Rubik regular 12, split in half with border */}
      <div
        className={`flex flex-col overflow-hidden ${CELL_BORDER}`}
        style={{ flex: `${F_TSAT} 0 0%`, height: "100%", minWidth: 0 }}
      >
        <div className="flex items-center gap-1 px-1 border-b border-[#85b4af]" style={{ height: HALF_H }}>
          <span className="shrink-0" style={{ fontFamily: RUBIK, fontWeight: 400, fontSize: 12 }}>TSAT</span>
          <span className="truncate" style={{ fontFamily: RUBIK, fontWeight: 400, fontSize: 12 }}>{tsat}</span>
        </div>
        <div className="flex items-center gap-1 px-1" style={{ height: HALF_H }}>
          <span className="shrink-0" style={{ fontFamily: RUBIK, fontWeight: 400, fontSize: 12 }}>CTOT</span>
          <span className="truncate" style={{ fontFamily: RUBIK, fontWeight: 400, fontSize: 12 }}>{ctot}</span>
        </div>
      </div>

      {/* RWY — 25%*(2/3)*(2/3), Rubik semibold 20 */}
      <div
        className="flex items-center justify-center overflow-hidden"
        style={{ flex: `${F_RWY} 0 0%`, height: "100%", paddingBottom: "1.48vh", minWidth: 0 }}
      >
        <span style={{ fontFamily: RUBIK, fontWeight: 600, fontSize: 20 }}>{runway}</span>
      </div>
    </div>
  );
}
