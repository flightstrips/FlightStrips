import { CLXBtn } from "@/components/clxbtn";
import { getStripBg } from "./types";
import type { StripProps } from "./types";
import { useSelectedCallsign, useSelectStrip } from "@/store/store-hooks";

const ARIAL = "'Arial', sans-serif";
const CELL_BORDER = "border-r border-[#85b4af]";
const FULL_H  = "4.44vh";
const HALF_H  = "2.22vh";

/**
 * ClxClearedStrip — used in the CLEARED bay on the CLX layout (status="CLROK").
 *
 * Same layout as DelStrip but with:
 *  - SI box (8.44% extra width → total 88.44% of bay)
 *  - CTOT added below EOBT
 *
 * Width: 88.44% of bay.
 *   SI (8.44) | Left 50% of 80: Callsign (2/3) + Dest/Stand (1/3)
 *             | Right 50% of 80: EOBT/CTOT (left) + TOBT/TSAT (right)
 */

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
      style={{ flex: "8.44 0 0%", height: "100%", backgroundColor: bgColor, minWidth: 0 }}
    >
      {nextLabel}
    </div>
  );
}

export function ClxClearedStrip({
  callsign,
  pdcStatus,
  destination,
  stand,
  eobt,
  tobt,
  tsat,
  ctot,
  arrival,
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
        height: FULL_H,
        width: "88.44%",
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
      <div
        className="flex text-black"
        style={{ height: "100%", overflow: "hidden", backgroundColor: getStripBg(pdcStatus, arrival) }}
      >
        {/* SI / ownership — 8.44% */}
        <SIBox
          owner={owner}
          nextControllers={nextControllers}
          previousControllers={previousControllers}
          myIdentifier={myIdentifier}
        />

        {/* ── Left half of 80% ── */}

        {/* Callsign — 2/3 of left half */}
        <button
          className={`flex items-center justify-start overflow-hidden active:bg-[#F237AA] ${CELL_BORDER}`}
          style={{ flex: "26.667 0 0%", height: "100%", minWidth: 0, fontFamily: ARIAL, fontWeight: "bold", fontSize: 24, textAlign: "left", paddingLeft: "4px" }}
        >
          <span className="truncate w-full">{callsign}</span>
        </button>

        {/* Dest / Stand — 1/3 of left half, top=dest bottom=stand, no line */}
        <div
          className={`flex flex-col overflow-hidden ${CELL_BORDER}`}
          style={{ flex: "13.333 0 0%", height: "100%", minWidth: 0 }}
        >
          <CLXBtn callsign={callsign}>
            <div className="flex items-center justify-center overflow-hidden" style={{ height: HALF_H, fontFamily: ARIAL, fontWeight: "bold", fontSize: 14 }}>
              {destination}
            </div>
            <div className="flex items-center justify-center overflow-hidden" style={{ height: HALF_H, fontFamily: ARIAL, fontWeight: "bold", fontSize: 14 }}>
              {stand}
            </div>
          </CLXBtn>
        </div>

        {/* ── Right half of 80% ── */}

        <div
          className="flex flex-row overflow-hidden"
          style={{ flex: "40 0 0%", height: "100%", minWidth: 0 }}
        >
          {/* EOBT / CTOT — left half, stacked with line between */}
          <div className={`flex flex-col overflow-hidden ${CELL_BORDER}`} style={{ flex: "1 0 0%", height: "100%", minWidth: 0 }}>
            <div className="flex items-center justify-between px-1 overflow-hidden" style={{ height: HALF_H, fontFamily: ARIAL, fontSize: 14 }}>
              <span className="text-black shrink-0">EOBT</span>
              <span>{eobt}</span>
            </div>
            <div className="flex items-center justify-between px-1 overflow-hidden" style={{ height: HALF_H, fontFamily: ARIAL, fontSize: 14 }}>
              <span className="text-black shrink-0">CTOT</span>
              <span>{ctot}</span>
            </div>
          </div>

          {/* TOBT / TSAT — right half, stacked with line between */}
          <div className="flex flex-col" style={{ flex: "1 0 0%", height: "100%" }}>
            <div className="flex items-center justify-between px-1 border-b border-[#85b4af] overflow-hidden" style={{ height: HALF_H, fontFamily: ARIAL, fontSize: 14 }}>
              <span className="text-black shrink-0">TOBT</span>
              <span>{tobt}</span>
            </div>
            <div className="flex items-center justify-between px-1 overflow-hidden" style={{ height: HALF_H, fontFamily: ARIAL, fontSize: 14 }}>
              <span className="text-black shrink-0">TSAT</span>
              <span>{tsat}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
