import { useState } from "react";
import { useLocation } from "react-router";
import Time from "@/components/Time";
import MRKBTN from "./MRKBTN";
import TRFBRN from "./TRFBRN";
import REQBTN from "./REQBTN";
import ATIS from "./ATIS";
import HOMEBTN from "./HOMEBTN";
import { useMetar } from "@/hooks/use-metar";
import MetarHelper from "@/components/MetarHelper";
import { useRunwaySetup, useSelectedCallsign, useSelectStrip, useWebSocketStore } from "@/store/store-hooks";
import { Bay } from "@/api/models";

const SCOPE_LABELS: Record<string, string> = {
  "/EKCH/CLX": "CLR DEL",
  "/EKCH/AAAD": "AA + AD",
  "/EKCH/GEGW": "GE / GW",
  "/EKCH/TWTE": "TW / TE",
};

export default function CommandBar() {
  const metar = useMetar("EKCH");
  const location = useLocation();
  const runwaySetup = useRunwaySetup();
  const selectedCallsign = useSelectedCallsign();
  const selectStrip = useSelectStrip();
  const move = useWebSocketStore((state) => state.move);

  const [markedCallsigns, setMarkedCallsigns] = useState<Set<string>>(new Set());
  const [unit, setUnit] = useState<"hPa" | "inHg">("hPa");

  const depRwy = runwaySetup.departure[0] ?? "—";
  const arrRwy = runwaySetup.arrival[0] ?? "—";

  const scopeLabel = SCOPE_LABELS[location.pathname] ?? location.pathname;

  const isMarked = !!selectedCallsign && markedCallsigns.has(selectedCallsign);

  const handleMark = () => {
    if (!selectedCallsign) return;
    setMarkedCallsigns((prev) => {
      const next = new Set(prev);
      if (next.has(selectedCallsign)) next.delete(selectedCallsign);
      else next.add(selectedCallsign);
      return next;
    });
  };

  const handleDelete = () => {
    if (!selectedCallsign) return;
    move(selectedCallsign, Bay.Hidden);
    selectStrip(null);
  };

  return (
    <div className="h-16 w-screen bg-[#3b3b3b] flex justify-between text-white">
      <div className="h-full w-full flex">
        <div className="bg-[#1bff16] text-black w-32 flex justify-center items-center m-2 font-bold">
          {scopeLabel}
        </div>
        <div className="flex w-32 text-2xl font-bold m-2 items-center justify-between">
          <h1>DEP</h1>
          <span className="bg-white text-black w-16 p-2">{depRwy}</span>
        </div>
        <div className="flex w-32 text-2xl font-bold m-2 items-center justify-between">
          <h1>ARR</h1>
          <span className="bg-white text-black w-16 p-2">{arrRwy}</span>
        </div>
        <div className="flex w-fit text-2xl font-bold m-2 items-center justify-between">
          <h1>QNH</h1>
          <span className="bg-[#212121] w-18 p-2">
            <MetarHelper metar={metar} style="qnh" unit={unit} />
          </span>
          <span
            className="bg-white text-black w-12 p-2 mx-2 text-center cursor-pointer select-none"
            onClick={() => setUnit((u) => (u === "hPa" ? "inHg" : "hPa"))}
          >
            D
          </span>
          <span className="bg-white text-black w-32 p-2 mx-2 text-center text-xl">
            <MetarHelper metar={metar} style="winds" />
          </span>
        </div>
        <div className="flex w-fit text-2xl font-bold m-2 items-center justify-between">
          <ATIS />
        </div>
      </div>
      <div className="flex items-center justify-center gap-1">
        <HOMEBTN />
        <TRFBRN />
        <MRKBTN isMarked={isMarked} disabled={!selectedCallsign} onClick={handleMark} />
        <REQBTN />
        <button
          disabled={!selectedCallsign}
          className={`bg-[#646464] text-xl font-bold p-2 border-2 ${!selectedCallsign ? "opacity-50 cursor-not-allowed" : ""}`}
          onClick={handleDelete}
        >
          X
        </button>
        <div className="w-32 bg-[#646464] flex items-center justify-center h-6/8 border-2">
          <Time />
        </div>
      </div>
    </div>
  );
}