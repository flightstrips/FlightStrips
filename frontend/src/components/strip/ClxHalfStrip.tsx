import { getSimpleAircraftType } from "@/lib/utils";
import type { StripProps } from "./types";


const CELL_BORDER = "border-r border-[#85b4af]";
const FONT = "'Arial', sans-serif";
// Flex-grow proportions (flex-basis: 0 so space is shared proportionally)
const F_SI       = 8;
const F_CALLSIGN = 25;
const F_TYPE     = 25 * (2 / 3);
const F_RUNWAY   = 25 * (1 / 2);
const F_SID      = 25 * (3 / 4);
const F_STAND    = 25 * (1 / 2);

const FONT_SIZE = 12;

export function ClxHalfStrip({
  callsign,
  aircraftType,
  runway,
  sid,
  stand
}: StripProps) {

  return (
    <div style={{ 
      height: "2.22vh",
      width: "80%",
      backgroundColor: "#85b4af",
      padding: "1px",
      borderLeft: "2px solid white",
      borderRight: "2px solid white",
      borderTop: "2px solid white",
      borderBottom: "2px solid white",
      boxShadow: "1px 0 0 0 #2F2F2F, 0 -1px 0 0 #2F2F2F",
     }}>

      <div className="flex text-black" style={{ height: "100%", overflow: "hidden", backgroundColor: "#bef5ef" }}>

        {/* OB — 8% */}
        <div className={`flex flex-col overflow-hidden ${CELL_BORDER}`} style={{ flex: `${F_SI} 0 0%`, height: "100%", minWidth: 0 }}>
          <div className="flex items-center justify-center" style={{ height: "100%" }}>
            <span className="font-bold" style={{ fontFamily: FONT, fontSize: FONT_SIZE }}>
              OB
            </span>
          </div>
        </div>

        {/* Callsign — 25% */}
        <div
          className={`flex flex-col overflow-hidden ${CELL_BORDER}`}
          style={{ flex: `${F_CALLSIGN} 0 0%`, height: "100%", minWidth: 0 }}
        >
          <div className="flex items-center pl-2" style={{ height: "100%" }}>
            <span className="truncate w-full" style={{ fontFamily: FONT, fontWeight: "bold", fontSize: FONT_SIZE }}>
              {callsign}
            </span>
          </div>
        </div>

        {/* Aircraft Type — 25% * 2/3 */}
        <div
          className={`flex flex-col overflow-hidden ${CELL_BORDER}`}
          style={{ flex: `${F_TYPE} 0 0%`, height: "100%", minWidth: 0 }}
        >
          <div className="flex items-center justify-center" style={{ height: "100%" }}>
            <span className="truncate" style={{ fontFamily: FONT, fontSize: FONT_SIZE }}>
              {getSimpleAircraftType(aircraftType)}
            </span>
          </div>
        </div>

        {/* Runway — 25% * 1/2 */}
        <div
          className={`flex flex-col overflow-hidden ${CELL_BORDER}`}
          style={{ flex: `${F_RUNWAY} 0 0%`, height: "100%", minWidth: 0 }}
        >
          <div className="flex items-center justify-center"  style={{ height: "100%" }}>
            <span style={{ fontFamily: FONT, fontWeight: "bold", fontSize: FONT_SIZE }}>
              {runway}
            </span>
          </div>
        </div>

        {/* SID — 25% * 3/4 */}
        <div
          className={`flex flex-col overflow-hidden ${CELL_BORDER}`}
          style={{ flex: `${F_SID} 0 0%`, height: "100%", minWidth: 0 }}
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
            <span className="truncate w-full" style={{ fontFamily: FONT, fontWeight: "bold", fontSize: FONT_SIZE }}>
              {stand}
            </span>
          </div>
        </div>

      </div>
    </div>
  )


}
