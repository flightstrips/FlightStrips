import type { StripProps } from "./types";
import { useSelectedCallsign, useSelectStrip } from "@/store/store-hooks";

const CELL_BORDER = "border-r border-[#85b4af]";
const RUBIK = "'Rubik', sans-serif";
const HALF_H = 24; // 48px strip split evenly for TSAT/CTOT

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
      className={`flex-shrink-0 flex items-center justify-center text-sm font-bold ${CELL_BORDER}`}
      style={{ width: 34, height: "100%", backgroundColor: bgColor }}
    >
      {nextLabel}
    </div>
  );
}

/**
 * ApnPushStrip — APNPUSH strip for STARTUP, PUSH BACK and DE-ICE bays (status="PUSH").
 *
 * 48px strip split evenly (24px / 24px):
 *   [34px SI] | [120px callsign] | [80px actype↑ / reg↓] | [80px stand] | [80px TSAT↑ / CTOT↓] | [54px RWY]
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
        height: 48,
        width: 448,
        backgroundColor: "#bef5ef",
        borderLeft: "2px solid white",
        borderRight: "2px solid white",
        borderTop: "2px solid white",
        borderBottom: "1px solid white",
        boxShadow: "1px 0 0 0 #2F2F2F, 0 -1px 0 0 #2F2F2F",
      }}
      onClick={handleClick}
    >
      {/* SI / ownership — 34px */}
      <SIBox
        owner={owner}
        nextControllers={nextControllers}
        previousControllers={previousControllers}
        myIdentifier={myIdentifier}
      />

      {/* Callsign — 120px, Rubik medium 20 */}
      <div className={`flex-shrink-0 flex items-center pl-2 ${CELL_BORDER}`} style={{ width: 120, height: "100%", paddingBottom: 16 }}>
        <span
          className="truncate w-full"
          style={{ fontFamily: RUBIK, fontWeight: 500, fontSize: 20 }}
        >
          {callsign}
        </span>
      </div>

      {/* A/C type / Registration — 80px, Rubik regular 10, stacked top 2/3 no divider */}
      <div className={`flex-shrink-0 flex flex-col items-center justify-center overflow-hidden ${CELL_BORDER}`} style={{ width: 80, height: "100%", paddingBottom: 16 }}>
        <span className="truncate px-1 leading-tight" style={{ fontFamily: RUBIK, fontWeight: 400, fontSize: 10 }}>{aircraftType?.split("/")[0]}</span>
        <span className="truncate px-1 leading-tight" style={{ fontFamily: RUBIK, fontWeight: 400, fontSize: 10 }}>OYFSR</span>
      </div>

      {/* Stand — 80px, Rubik semibold 20 */}
      <div className={`flex-shrink-0 flex items-center justify-center ${CELL_BORDER}`} style={{ width: 80, height: "100%", paddingBottom: 16 }}>
        <span style={{ fontFamily: RUBIK, fontWeight: 600, fontSize: 20 }}>{stand}</span>
      </div>

      {/* TSAT / CTOT — 80px, Rubik regular 12, split in half with border */}
      <div className={`flex-shrink-0 flex flex-col ${CELL_BORDER}`} style={{ width: 80, height: "100%" }}>
        <div className="flex items-center gap-1 px-1 border-b border-[#85b4af]" style={{ height: HALF_H }}>
          <span className="shrink-0" style={{ fontFamily: RUBIK, fontWeight: 400, fontSize: 12 }}>TSAT</span>
          <span className="truncate" style={{ fontFamily: RUBIK, fontWeight: 400, fontSize: 12 }}>{tsat}</span>
        </div>
        <div className="flex items-center gap-1 px-1" style={{ height: HALF_H }}>
          <span className="shrink-0" style={{ fontFamily: RUBIK, fontWeight: 400, fontSize: 12 }}>CTOT</span>
          <span className="truncate" style={{ fontFamily: RUBIK, fontWeight: 400, fontSize: 12 }}>{ctot}</span>
        </div>
      </div>

      {/* RWY — 54px, Rubik semibold 20 */}
      <div className="flex-shrink-0 flex items-center justify-center overflow-hidden" style={{ width: 54, height: "100%", paddingBottom: 16 }}>
        <span style={{ fontFamily: RUBIK, fontWeight: 600, fontSize: 20 }}>{runway}</span>
      </div>
    </div>
  );
}
