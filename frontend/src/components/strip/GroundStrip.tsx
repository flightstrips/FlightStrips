import { StripCell, SplitStripCell } from "./StripCell";
import { getStripBg } from "./types";
import type { StripProps } from "./types";
import { useSelectedCallsign, useSelectStrip } from "@/store/store-hooks";

// -----------------------------------------------------------------------------
// SIBox — rightmost 38px cell showing strip ownership state
// -----------------------------------------------------------------------------

interface SIBoxProps {
  owner?: string;
  nextControllers?: string[];
  previousControllers?: string[];
  myIdentifier?: string;
}

function SIBox({ owner, nextControllers, previousControllers, myIdentifier }: SIBoxProps) {
  const isAssumed = !!myIdentifier && owner === myIdentifier;
  const isTransferredAway =
    !!myIdentifier &&
    !!previousControllers?.includes(myIdentifier) &&
    !nextControllers?.includes(myIdentifier);

  let bgColor = "#E082E7"; // Purple — Concerned (default)
  if (isAssumed) bgColor = "#F0F0F0";
  else if (isTransferredAway) bgColor = "#DD6A12";

  // In Assumed state, display 2-letter abbreviation of next controller
  const nextLabel =
    isAssumed && nextControllers?.[0] ? nextControllers[0].slice(0, 2) : "";

  return (
    <div
      className="flex-shrink-0 flex items-center justify-center h-full text-sm font-bold"
      style={{ width: 38, backgroundColor: bgColor }}
    >
      {nextLabel}
    </div>
  );
}

// -----------------------------------------------------------------------------
// GroundStrip — shown after clearance is issued (status="CLROK")
// -----------------------------------------------------------------------------

/**
 * GroundStrip - shown after clearance is issued (status="CLROK").
 */
export function GroundStrip({
  callsign,
  pdcStatus,
  destination,
  stand,
  eobt,
  tobt,
  tsat,
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
      className={`flex h-[42px] w-fit text-black select-none${isSelected ? " outline outline-2 outline-[#FF00F5]" : ""}${selectable ? " cursor-pointer" : ""}`}
      style={{
        backgroundColor: getStripBg(pdcStatus, arrival),
        borderLeft: "2px solid white",
        borderRight: "2px solid white",
        borderTop: "2px solid white",
        borderBottom: "1px solid white",
        boxShadow: "1px 0 0 0 #2F2F2F, 0 -1px 0 0 #2F2F2F",
      }}
      onClick={handleClick}
    >
      {/* SI Box — ownership state indicator */}
      <SIBox
        owner={owner}
        nextControllers={nextControllers}
        previousControllers={previousControllers}
        myIdentifier={myIdentifier}
      />

      {/* Callsign */}
      <StripCell width={130} className="flex items-center">
        <button className="w-full h-8 text-left pl-2 font-bold text-xl active:bg-[#F237AA] truncate">
          {callsign}
        </button>
      </StripCell>

      {/* Destination / Stand */}
      <SplitStripCell
        width={65}
        top={
          <span className="px-1 text-sm font-semibold truncate w-full text-center">
            {destination}
          </span>
        }
        bottom={
          <span className="px-1 text-xs truncate w-full text-center">
            {stand}
          </span>
        }
      />

      {/* EOBT */}
      <StripCell
        width={90}
        className="flex items-center justify-between px-1"
      >
        <span className="text-xs text-gray-600 shrink-0">EOBT</span>
        <span className="text-xs font-medium">{eobt}</span>
      </StripCell>

      {/* TOBT / TSAT stacked */}
      <SplitStripCell
        width={90}
        top={
          <div className="flex justify-between w-full px-1">
            <span className="text-xs text-gray-600">TOBT</span>
            <span className="text-xs font-medium">{tobt}</span>
          </div>
        }
        bottom={
          <div className="flex justify-between w-full px-1">
            <span className="text-xs text-gray-600">TSAT</span>
            <span className="text-xs font-medium">{tsat}</span>
          </div>
        }
      />
    </div>
  );
}