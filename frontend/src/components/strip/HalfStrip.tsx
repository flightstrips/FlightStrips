import type { StripProps } from "./types";

/**
 * HalfStrip – compact single-row strip used in pushback/taxi bays (status="HALF").
 *
 * Layout (left → right):
 *  [Indicator] [Callsign] [Type] [Runway] [Taxiway] [HP] [Stand]
 */
export function HalfStrip({
  callsign,
  aircraftType,
  runway,
  taxiway,
  holdingPoint,
  stand,
}: StripProps) {
  return (
    <div className="w-full h-8 bg-[#bfbfbf] border border-[#d9d9d9] outline outline-1 outline-white text-black flex text-sm">
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
