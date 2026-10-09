import type { TacticalStrip } from "@/api/models";
import { getDeiceHeaderDisplayLabel } from "@/lib/deiceLane";
import { useMyPosition, useWebSocketStore } from "@/store/store-hooks";
import { CLS_LABEL, FONT } from "./shared";

interface Props {
  strip: TacticalStrip;
  width?: string | number;
}

export function TacticalDeiceLaneStrip({ strip, width }: Props) {
  const myPosition = useMyPosition();
  const deleteTacticalStrip = useWebSocketStore((state) => state.deleteTacticalStrip);
  const canDelete = strip.owner === myPosition;

  return (
    <div
      className="bay-col-header bay-col-sep !h-[2.775dvh] !z-0 !px-0"
      style={{ width: width ?? "100%" }}
    >
      <div
        className="flex flex-1 items-center justify-start overflow-hidden px-[0.42vw]"
        style={{ fontFamily: FONT }}
      >
        <span className={`${CLS_LABEL} truncate`}>{getDeiceHeaderDisplayLabel(strip)}</span>
      </div>
      {canDelete && (
        <button
          type="button"
          aria-label={`Close ${getDeiceHeaderDisplayLabel(strip)}`}
          className="flex h-full w-[1.9vw] shrink-0 items-center justify-center text-[#CECECE]"
          onClick={(event) => {
            event.stopPropagation();
            deleteTacticalStrip(strip.id);
          }}
        >
          <span className="flex h-[1.7dvh] w-[1.7dvh] items-center justify-center rounded-[0.4dvh] border border-current text-[1.1dvh] leading-none">
            X
          </span>
        </button>
      )}
    </div>
  );
}
