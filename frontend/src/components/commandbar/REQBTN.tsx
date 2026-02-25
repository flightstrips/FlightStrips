import { useSelectedCallsign, usePosition, useStrip, useWebSocketStore } from "@/store/store-hooks";
import { Bay } from "@/api/models";

export default function REQBTN() {
  const selectedCallsign = useSelectedCallsign();
  const position = usePosition();
  const strip = useStrip(selectedCallsign ?? "");
  const move = useWebSocketStore((state) => state.move);

  const canReq = !!selectedCallsign && strip?.owner !== position;

  return (
    <button
      disabled={!canReq}
      className={`bg-[#646464] text-xl font-bold p-2 border-2 ${!canReq ? "opacity-50 cursor-not-allowed" : ""}`}
      onClick={() => canReq && move(selectedCallsign!, Bay.Unknown)}
    >
      REQ
    </button>
  );
}