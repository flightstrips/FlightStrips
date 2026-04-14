import { useEffect, useMemo, useRef, useState } from "react";
import EstStandCell from "@/components/est/EstStandCell";
import EstViewButtons from "@/components/est/EstViewButtons";
import {
  EST_BACKGROUND_BOXES,
  EST_BOARD_HEIGHT,
  EST_BOARD_WIDTH,
  getEstStandsForView,
  isCargoStand,
  type EstView,
} from "@/components/est/metadata";
import { Bay, type FrontendStrip } from "@/api/models";
import { useStrips, useWebSocketStore } from "@/store/store-hooks";

const COLOR_LABEL_DEFAULT = "#202020";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  callsign: string;
  currentStand?: string;
}

export function ArrStandDialog({ open, onOpenChange, callsign, currentStand }: Props) {
  const updateStrip = useWebSocketStore(s => s.updateStrip);
  const strips = useStrips();
  const [boardScale, setBoardScale] = useState(1);
  const [boardViewOverride, setBoardViewOverride] = useState<EstView | null>(null);
  const boardFrameRef = useRef<HTMLDivElement>(null);
  const [nowMs] = useState(() => Date.now());
  const defaultBoardView: EstView = currentStand && isCargoStand(currentStand) ? "CARGO" : "MAIN";
  const boardView = boardViewOverride ?? defaultBoardView;

  useEffect(() => {
    if (!open) {
      return undefined;
    }

    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setBoardViewOverride(null);
        onOpenChange(false);
      }
    };
    window.addEventListener("keydown", handleEscape);
    return () => window.removeEventListener("keydown", handleEscape);
  }, [open, onOpenChange]);

  useEffect(() => {
    if (!open) {
      return undefined;
    }

    const element = boardFrameRef.current;
    if (!element) {
      return undefined;
    }

    const updateScale = () => {
      const { width, height } = element.getBoundingClientRect();
      if (!width || !height) {
        return;
      }
      setBoardScale(Math.min(width / EST_BOARD_WIDTH, height / EST_BOARD_HEIGHT));
    };

    updateScale();

    const observer = new ResizeObserver(updateScale);
    observer.observe(element);
    window.addEventListener("resize", updateScale);

    return () => {
      observer.disconnect();
      window.removeEventListener("resize", updateScale);
    };
  }, [open]);

  const stripByStand = useMemo(() => {
    const mapping = new Map<string, FrontendStrip>();
    for (const strip of strips) {
      if (!strip.stand || strip.bay === Bay.Hidden || strip.bay === Bay.ArrHidden) {
        continue;
      }
      mapping.set(strip.stand, strip);
    }
    return mapping;
  }, [strips]);
  const visibleStands = useMemo(() => getEstStandsForView(boardView), [boardView]);

  if (!open) {
    return null;
  }

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      setBoardViewOverride(null);
    }

    onOpenChange(nextOpen);
  }

  function handleStandClick(stand: string) {
    updateStrip(callsign, { stand });
    handleOpenChange(false);
  }

  return (
    <div className="fixed inset-0 z-50 bg-[#767676]" onMouseDown={() => handleOpenChange(false)}>
      <div
        ref={boardFrameRef}
        className="relative h-full w-full overflow-hidden"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div
          className="absolute left-1/2 top-1/2"
          style={{
            width: EST_BOARD_WIDTH * boardScale,
            height: EST_BOARD_HEIGHT * boardScale,
            transform: "translate(-50%, -50%)",
          }}
        >
          <div
            className="relative origin-top-left"
            style={{
              width: EST_BOARD_WIDTH,
              height: EST_BOARD_HEIGHT,
              transform: `scale(${boardScale})`,
            }}
          >
            {boardView !== "CARGO" && EST_BACKGROUND_BOXES.map((box) => (
              <div
                key={`${box.x}-${box.y}`}
                className="absolute flex items-center justify-center font-bold"
                style={{
                  left: box.x,
                  top: box.y,
                  width: box.width,
                  height: box.height,
                  borderRadius: box.radius ?? 0,
                  backgroundColor: box.fill,
                  color: box.labelColor ?? COLOR_LABEL_DEFAULT,
                  fontSize: box.label ? 32 : undefined,
                }}
                >
                  {box.label}
                </div>
              ))}

            <EstViewButtons
              view={boardView}
              onViewChange={(nextView) => setBoardViewOverride(nextView === defaultBoardView ? null : nextView)}
            />

            {visibleStands.map((stand) => {
              const strip = stripByStand.get(stand.label);
              const isCurrent = stand.label === currentStand;

              return (
                <EstStandCell
                  key={`${stand.label}-${stand.left}-${stand.top}`}
                  stand={stand}
                  strip={strip}
                  selected={isCurrent}
                  blocked={false}
                  actionActive={isCurrent}
                  blinking={false}
                  ctotImproved={false}
                  nowMs={nowMs}
                  containerStyle={{
                    position: "absolute",
                    left: stand.left,
                    top: stand.top,
                  }}
                  onClick={(standLabel) => handleStandClick(standLabel)}
                />
              );
            })}
          </div>
        </div>
      </div>
    </div>
  );
}
