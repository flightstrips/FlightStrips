import { useEffect, useMemo, useRef, useState } from "react";
import EsetStandCell from "@/components/eset/EsetStandCell";
import EsetViewButtons from "@/components/eset/EsetViewButtons";
import {
  ESET_BACKGROUND_BOXES,
  ESET_BOARD_HEIGHT,
  ESET_BOARD_WIDTH,
  getEsetStandsForView,
  isCargoStand,
  type EsetView,
} from "@/components/eset/metadata";
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
  const [boardViewOverride, setBoardViewOverride] = useState<EsetView | null>(null);
  const boardFrameRef = useRef<HTMLDivElement>(null);
  const [nowMs] = useState(() => Date.now());
  const defaultBoardView: EsetView = currentStand && isCargoStand(currentStand) ? "CARGO" : "MAIN";
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
      setBoardScale(Math.min(width / ESET_BOARD_WIDTH, height / ESET_BOARD_HEIGHT));
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
  const visibleStands = useMemo(() => getEsetStandsForView(boardView), [boardView]);

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
            width: ESET_BOARD_WIDTH * boardScale,
            height: ESET_BOARD_HEIGHT * boardScale,
            transform: "translate(-50%, -50%)",
          }}
        >
          <div
            className="relative origin-top-left"
            style={{
              width: ESET_BOARD_WIDTH,
              height: ESET_BOARD_HEIGHT,
              transform: `scale(${boardScale})`,
            }}
          >
            {ESET_BACKGROUND_BOXES.map((box) => (
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

            <EsetViewButtons
              view={boardView}
              onViewChange={(nextView) => setBoardViewOverride(nextView === defaultBoardView ? null : nextView)}
            />

            {visibleStands.map((stand) => {
              const strip = stripByStand.get(stand.label);
              const isCurrent = stand.label === currentStand;

              return (
                <EsetStandCell
                  key={stand.label}
                  stand={stand}
                  strip={strip}
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
