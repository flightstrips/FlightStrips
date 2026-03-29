import React from "react";
import { useControllers, useWebSocketStore } from "@/store/store-hooks";
import { getCellBorderColor, FONT, COLOR_BTN_ORANGE, COLOR_SI_ASSUMED, COLOR_SI_UNCONCERNED, COLOR_SI_CONCERNED, getStripOwnership } from "./shared";

/** Text colour for the next-controller identifier label. */
const COLOR_SI_LABEL = "#8F8F8F";

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
  isTagRequest,
  baseBorderColor,
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
  /** True when the active coordination is a tag request (REQ) rather than a normal transfer. */
  isTagRequest?: boolean;
  /** Base cell border color (defaults to the shared teal). Pass a custom color for strips with different border styling. */
  baseBorderColor?: string;
}) {
  const controllers = useControllers();
  const transferStrip = useWebSocketStore(s => s.transferStrip);
  const assumeStrip = useWebSocketStore(s => s.assumeStrip);
  const cancelTransfer = useWebSocketStore(s => s.cancelTransfer);
  const acceptTagRequest = useWebSocketStore(s => s.acceptTagRequest);

  const { isAssumed, isTransferredAway, isConcerned } = getStripOwnership(myPosition, owner, nextControllers, previousControllers);

  const isSendingTransfer = isAssumed && !!transferringTo && !isTagRequest;
  const isReceivingTransfer = !!myPosition && !!transferringTo && transferringTo === myPosition && !isAssumed && !isTagRequest;
  const isUnownedAndNext = !owner && isConcerned;

  // Tag request states
  const isTagRequestOwner = isAssumed && !!transferringTo && isTagRequest;
  const isTagRequestRequester = !!myPosition && !!transferringTo && transferringTo === myPosition && !isAssumed && isTagRequest;

  const nextPosition = nextControllers?.find(pos => pos !== myPosition);

  let nextLabel = "";
  if (isSendingTransfer || isTagRequestOwner) {
    const targetController = controllers.find(c => c.position === transferringTo);
    nextLabel = targetController ? targetController.identifier : "";
  } else if (isAssumed && !isTagRequestOwner) {
    const nextController = controllers.find(c => c.position === nextPosition);
    nextLabel = nextController ? nextController.identifier : "";
  }

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (isTagRequestOwner) {
      acceptTagRequest(callsign);
    } else if (isReceivingTransfer || isUnownedAndNext) {
      assumeStrip(callsign);
    } else if (isSendingTransfer) {
      cancelTransfer(callsign);
    } else if (isAssumed && nextPosition) {
      transferStrip(callsign, nextPosition);
    }
  };

  const isClickable = isTagRequestOwner || isReceivingTransfer || isUnownedAndNext || isSendingTransfer || (isAssumed && !!nextPosition && !isTagRequestOwner);

  let background: string;
  if (isTagRequestOwner) {
    // Owner side of a tag request: white+orange gradient (sending-away appearance)
    background = `linear-gradient(to right, ${COLOR_SI_ASSUMED} 50%, ${COLOR_BTN_ORANGE} 50%)`;
  } else if (isTagRequestRequester) {
    // Unassumed tag requests keep the normal concerned appearance rather than the split transfer layout.
    background = COLOR_SI_CONCERNED;
  } else if (isSendingTransfer) {
    background = `linear-gradient(to right, ${COLOR_SI_ASSUMED} 50%, ${COLOR_BTN_ORANGE} 50%)`;
  } else if (isReceivingTransfer) {
    background = `linear-gradient(to right, ${COLOR_SI_CONCERNED} 50%, ${COLOR_SI_ASSUMED} 50%)`;
  } else if (isAssumed) {
    background = COLOR_SI_ASSUMED;
  } else if (isTransferredAway) {
    background = COLOR_BTN_ORANGE;
  } else if (isConcerned) {
    background = COLOR_SI_CONCERNED;
  } else {
    background = COLOR_SI_UNCONCERNED;
  }

  return (
    <div
      className="flex items-center justify-center font-bold border-r-2"
      style={{
        flex: `${flexGrow} 0 0%`,
        height: "100%",
        background: background,
        minWidth: 0,
        borderRightColor: getCellBorderColor(!!marked, baseBorderColor),
        fontFamily: FONT,
        fontSize: "1.15vw",
        color: COLOR_SI_LABEL,
        cursor: isClickable ? "pointer" : "default",
      }}
      onClick={isClickable ? handleClick : undefined}
    >
      {nextLabel}
    </div>
  );
}
