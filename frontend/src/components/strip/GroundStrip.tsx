import { StripCell, SplitStripCell } from "./StripCell";
import { getStripBg } from "./types";
import type { StripProps } from "./types";
import { useSelectedCallsign, useSelectStrip } from "@/store/store-hooks";

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
  selectable,
}: StripProps) {
  const selectedCallsign = useSelectedCallsign();
  const selectStrip = useSelectStrip();
  const isSelected = selectable && selectedCallsign === callsign;

  const handleClick = selectable
    ? () => selectStrip(isSelected ? null : callsign)
    : undefined;

  if (arrival) {
    return <div className="w-full h-12 bg-[#fff28e]" />;
  }

  return (
    <div
      className={`flex h-12 w-fit border border-[#85b4af] outline outline-1 text-black select-none${isSelected ? " outline-[#FF00F5]" : " outline-white"}${selectable ? " cursor-pointer" : ""}`}
      style={{ backgroundColor: getStripBg(pdcStatus) }}
      onClick={handleClick}
    >
      {/* Current owner position indicator */}
      <StripCell
        width={40}
        className="flex items-center justify-center font-bold text-sm bg-orange-500 text-white"
      >
        {owner ?? ""}
      </StripCell>

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
