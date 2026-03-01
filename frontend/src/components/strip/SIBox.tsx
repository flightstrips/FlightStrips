import { useControllers, useWebSocketStore } from "@/store/store-hooks";
import { getCellBorderColor } from "./shared";

const F_SI = 8;

/** SI / ownership indicator. Purple = unassumed, white = assumed, orange = transferred away. */
export function SIBox({
  callsign,
  owner,
  nextControllers,
  previousControllers,
  myPosition,
  marked,
  flexGrow = F_SI,
  transferringTo,
}: {
  callsign: string;
  owner?: string;
  nextControllers?: string[];
  previousControllers?: string[];
  myPosition?: string;
  /** Pass true when the strip is in the marked state (not yet wired). */
  marked?: boolean;
  /** flex-grow value (defaults to 8). Override when the strip layout requires a different proportion. */
  flexGrow?: number;
  /** Position string of the controller being transferred to. Empty string or undefined means no transfer. */
  transferringTo?: string;
}) {
  const controllers = useControllers();
  const transferStrip = useWebSocketStore(s => s.transferStrip);
  const assumeStrip = useWebSocketStore(s => s.assumeStrip);
  const freeStrip = useWebSocketStore(s => s.freeStrip);

  const isAssumed = !!myPosition && owner === myPosition;
  const isTransferredAway = !!myPosition && !!previousControllers?.includes(myPosition);
  const isConcerned = !!myPosition && !!nextControllers?.includes(myPosition);

  const isSendingTransfer = isAssumed && !!transferringTo;
  const isReceivingTransfer = !!myPosition && !!transferringTo && transferringTo === myPosition && !isAssumed;

  const nextPosition = nextControllers?.find(pos => pos !== myPosition);

  let nextLabel = "";
  if (isSendingTransfer) {
    const targetController = controllers.find(c => c.position === transferringTo);
    nextLabel = targetController ? targetController.identifier : "";
  } else if (isAssumed) {
    const nextController = controllers.find(c => c.position === nextPosition);
    nextLabel = nextController ? nextController.identifier : "";
  }

  const handleClick = () => {
    if (isReceivingTransfer) {
      assumeStrip(callsign);
    } else if (isSendingTransfer) {
      freeStrip(callsign);
    } else if (isAssumed && nextPosition) {
      transferStrip(callsign, nextPosition);
    }
  };

  const isClickable = isReceivingTransfer || isSendingTransfer || (isAssumed && !!nextPosition);

  let background: string;
  if (isSendingTransfer) {
    background = "linear-gradient(to right, #F0F0F0 50%, #DD6A12 50%)";
  } else if (isReceivingTransfer) {
    background = "linear-gradient(to right, #E082E7 50%, #F0F0F0 50%)";
  } else if (isAssumed) {
    background = "#F0F0F0";
  } else if (isTransferredAway) {
    background = "#DD6A12";
  } else if (isConcerned) {
    background = "#E082E7";
  } else {
    background = "#808080";
  }

  return (
    <div
      className="flex items-center justify-center text-sm font-bold border-r-2"
      style={{
        flex: `${flexGrow} 0 0%`,
        height: "100%",
        background: background,
        minWidth: 0,
        borderRightColor: getCellBorderColor(!!marked),
        fontFamily: "'Arial', sans-serif",
        fontSize: 22,
        color: "#8F8F8F",
        cursor: isClickable ? "pointer" : "default",
      }}
      onClick={isClickable ? handleClick : undefined}
    >
      {nextLabel}
    </div>
  );
}
