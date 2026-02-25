import type { StripProps } from "./types";
import { useSelectedCallsign, useSelectStrip } from "@/store/store-hooks";

/**
 * HalfStrip - compact single-row strip used in pushback/taxi bays (status="HALF").
 */
export function HalfStrip({
  callsign,
  aircraftType,
  runway,
  taxiway,
  holdingPoint,
  stand,
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
      className={`w-full h-8 bg-[#bfbfbf] border border-[#d9d9d9] outline outline-1 text-black flex text-sm${isSelected ? " outline-[#FF00F5]" : " outline-white"}${selectable ? " cursor-pointer" : ""}`}
      onClick={handleClick}
    >
      <div className="h-full w-8 border border-[#d9d9d9] flex items-center justify-center font-bold text-xs">
        OB
      </div>
      <div className="h-full flex-1 border border-[#d9d9d9] flex items-center pl-2 font-bold truncate">
        {callsign}
      </div>
      <div className="h-full w-14 border border-[#d9d9d9] flex items-center justify-center text-xs">
        {aircraftType}
      </div>
      <div className="h-full w-14 border border-[#d9d9d9] flex items-center justify-center font-bold">
        {runway}
      </div>
      <div className="h-full w-14 border border-[#d9d9d9] flex items-center justify-center font-bold">
        {taxiway}
      </div>
      <div className="h-full w-10 border border-[#d9d9d9] flex items-center justify-center text-xs">
        {holdingPoint}
      </div>
      <div className="h-full w-14 border border-[#d9d9d9] flex items-center justify-center font-bold">
        {stand}
      </div>
    </div>
  );
}
