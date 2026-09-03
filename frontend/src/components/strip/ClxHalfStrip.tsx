import type { StripProps } from "./types";
import { getStripBg } from "./types";
import { AircraftTypeLabel, FONT, getFramedStripStyle, getSIBoxBorderStyle, getStripFrameColor, useStripBg } from "./shared";
// Flex-grow proportions (flex-basis: 0 so space is shared proportionally)
const F_SI       = 8;
const F_CALLSIGN = 25;
const F_TYPE     = 25 * (2 / 3);
const F_RUNWAY   = 25 * (1 / 2);
const F_SID      = 25 * (3 / 4);
const F_STAND    = 25 * (1 / 2);

const FONT_SIZE = "0.63vw";

export function ClxHalfStrip({
  callsign,
  bay,
  aircraftType,
  aircraftCategory,
  runway,
  sid,
  stand,
  pdcStatus,
  arrival,
  fullWidth
}: StripProps) {
  const { bg, textWhite } = useStripBg(runway, getStripBg(pdcStatus, arrival, bay), false, false, pdcStatus, bay);
  const stripFrameColor = getStripFrameColor(arrival ?? false);

  return (
    <div style={{
      height: "2.36dvh",
      width: fullWidth ? "100%" : "80%",
      ...getFramedStripStyle(false, stripFrameColor),
     }}>

      <div className={`flex ${textWhite ? "text-white" : "text-black"}`} style={{ height: "100%", overflow: "hidden", backgroundColor: bg }}>

        {/* OB — 8% */}
        <div className="flex flex-col overflow-hidden" style={{ flex: `${F_SI} 0 0%`, height: "100%", minWidth: 0, ...getSIBoxBorderStyle(false, stripFrameColor) }}>
          <div className="flex items-center justify-center" style={{ height: "100%" }}>
            <span className="font-bold" style={{ fontFamily: FONT, fontSize: FONT_SIZE }}>
              OB
            </span>
          </div>
        </div>

        {/* Callsign — 25% */}
        <div
          className="flex flex-col overflow-hidden border-r-2"
          style={{ flex: `${F_CALLSIGN} 0 0%`, height: "100%", minWidth: 0, borderRightColor: stripFrameColor }}
        >
          <div className="flex items-center pl-[0.42vw]" style={{ height: "100%" }}>
            <span className="truncate w-full" style={{ fontFamily: FONT, fontWeight: "bold", fontSize: FONT_SIZE }}>
              {callsign}
            </span>
          </div>
        </div>

        {/* Aircraft Type — 25% * 2/3 */}
        <div
          className="flex flex-col overflow-hidden border-r-2"
          style={{ flex: `${F_TYPE} 0 0%`, height: "100%", minWidth: 0, borderRightColor: stripFrameColor }}
        >
          <div className="flex items-center justify-center" style={{ height: "100%" }}>
            <AircraftTypeLabel className="truncate" style={{ fontFamily: FONT, fontSize: FONT_SIZE }} aircraftType={aircraftType} aircraftCategory={aircraftCategory} />
          </div>
        </div>

        {/* Runway — 25% * 1/2 */}
        <div
          className="flex flex-col overflow-hidden border-r-2"
          style={{ flex: `${F_RUNWAY} 0 0%`, height: "100%", minWidth: 0, borderRightColor: stripFrameColor }}
        >
          <div className="flex items-center justify-center"  style={{ height: "100%" }}>
            <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: FONT_SIZE }}>
              {runway}
            </span>
          </div>
        </div>

        {/* SID — 25% * 3/4 */}
        <div
          className="flex flex-col overflow-hidden border-r-2"
          style={{ flex: `${F_SID} 0 0%`, height: "100%", minWidth: 0, borderRightColor: stripFrameColor }}
        >
          <div className="flex items-center justify-center" style={{ height: "100%" }}>
            <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: FONT_SIZE }}>
              {sid}
            </span>
          </div>
        </div>

        {/* Stand — 25% * 1/2 */}
        <div
          className={`flex flex-col overflow-hidden`}
          style={{ flex: `${F_STAND} 0 0%`, height: "100%", minWidth: 0 }}
        >
          <div className="flex items-center justify-center" style={{ height: "100%" }}>
            <span className="truncate" style={{ fontFamily: FONT, fontWeight: "bold", fontSize: FONT_SIZE }}>
              {stand}
            </span>
          </div>
        </div>

      </div>
    </div>
  )


}
