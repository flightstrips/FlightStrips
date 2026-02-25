import { CLXBtn } from "@/components/clxbtn";
import { StripCell, SplitStripCell } from "./StripCell";
import { getStripBg } from "./types";
import type { StripProps } from "./types";
import { useSelectedCallsign, useSelectStrip } from "@/store/store-hooks";

/**
 * DelStrip - shown before departure clearance is issued (status="CLR").
 */
export function DelStrip({
  callsign,
  pdcStatus,
  destination,
  stand,
  eobt,
  tobt,
  tsat,
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
      className={`flex h-12 w-fit border border-[#85b4af] outline outline-1 text-black select-none${isSelected ? " outline-[#FF00F5]" : " outline-white"}${selectable ? " cursor-pointer" : ""}`}
      style={{ backgroundColor: getStripBg(pdcStatus) }}
      onClick={handleClick}
    >
      {/* Callsign */}
      <StripCell width={130} className="flex items-center">
        <button className="w-full h-8 text-left pl-2 font-bold text-xl active:bg-[#F237AA] truncate">
          {callsign}
        </button>
      </StripCell>

      {/* Destination / Stand → opens flight-plan clearance dialog */}
      <StripCell width={65} className="flex items-center justify-center">
        <CLXBtn callsign={callsign}>
          <div className="flex flex-col items-center justify-center leading-tight w-full">
            <span className="font-semibold text-sm">{destination}</span>
            <span className="text-xs">{stand}</span>
          </div>
        </CLXBtn>
      </StripCell>

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
