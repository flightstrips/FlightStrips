import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
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
import { ActionType, Bay, type FrontendStrip } from "@/api/models";
import { useStrips, useWebSocketStore } from "@/store/store-hooks";

const COLOR_LABEL_DEFAULT = "#202020";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  callsign: string;
  currentStand?: string;
}

export function ArrStandDialog({ open, onOpenChange, callsign, currentStand }: Props) {
  const satEnabled = useWebSocketStore(s => s.satEnabled);
  return satEnabled
    ? <SatStandAssignmentMenu open={open} onOpenChange={onOpenChange} callsign={callsign} />
    : <LegacyArrStandDialog open={open} onOpenChange={onOpenChange} callsign={callsign} currentStand={currentStand} />;
}

function SatStandAssignmentMenu({ open, onOpenChange, callsign }: Omit<Props, "currentStand">) {
  const [manualStand, setManualStand] = useState("");
  const assignment = useWebSocketStore(s => s.standAssignments.find(a => a.callsign === callsign));
  const rejection = useWebSocketStore(s => s.standActionRejection);
  const requestAutomatic = useWebSocketStore(s => s.requestAutomaticStand);
  const requestManual = useWebSocketStore(s => s.requestManualStand);
  const confirmOverride = useWebSocketStore(s => s.confirmStandOverride);
  const clearRejection = useWebSocketStore(s => s.clearStandActionRejection);
  const submittedVersion = useRef<number | null>(null);
  const version = assignment?.version ?? 0;
  const relevantRejection = rejection?.callsign === callsign ? rejection : null;
  const unsafeManual = relevantRejection?.action === ActionType.FrontendStandAssignmentManualRequest
    && relevantRejection.code === "incompatible_or_occupied";

  useEffect(() => {
    if (open && submittedVersion.current !== null && assignment?.version !== submittedVersion.current) {
      submittedVersion.current = null;
      onOpenChange(false);
    }
  }, [assignment?.version, onOpenChange, open]);

  if (!open) return null;

  const close = () => {
    setManualStand("");
    submittedVersion.current = null;
    clearRejection();
    onOpenChange(false);
  };
  const send = () => {
    submittedVersion.current = version;
    const stand = manualStand.trim().toUpperCase();
    if (stand) requestManual(callsign, stand, version);
    else requestAutomatic(callsign, version);
  };

  if (relevantRejection) {
    return (
      <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/25" onMouseDown={close}>
        <div className="w-[38rem] border border-black bg-[#e4e4e4] p-5 text-center shadow-lg" onMouseDown={e => e.stopPropagation()} role="alertdialog" aria-label="Stand assignment warning">
          <h2 className="mb-5 text-2xl font-light">STAND ASSIGNMENT</h2>
          <p className="mb-6 text-2xl text-red-600">{relevantRejection.code === "invalid_stand" ? "STAND NOT FOUND" : relevantRejection.reason}</p>
          <div className="flex justify-center gap-4">
            <MenuButton onClick={close}>ESC</MenuButton>
            {unsafeManual && <MenuButton onClick={() => { submittedVersion.current = version; requestAutomatic(callsign, version); }}>AUTO ASSIGN</MenuButton>}
            {unsafeManual && <MenuButton onClick={() => { submittedVersion.current = version; confirmOverride(callsign, manualStand.trim().toUpperCase(), version, relevantRejection.reason); }}>YES</MenuButton>}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/20" onMouseDown={close}>
      <div className="w-[17.8rem] border border-black bg-[#b3b3b3] p-4 shadow-lg" onMouseDown={e => e.stopPropagation()} role="dialog" aria-label="Stand assignment">
        <h2 className="mb-2 text-center text-2xl font-light">STAND ASSIGNMENT</h2>
        <div className="flex flex-col gap-2">
          <MenuButton onClick={send}>SEND REQ</MenuButton>
          <button className={`h-[4.4rem] text-3xl ${manualStand ? "bg-[#3f3f3f] text-white" : "bg-[#00ff26] text-black"}`} onClick={() => setManualStand("")}>AUTOMATIC</button>
          <input
            className="h-[4.4rem] bg-[#fcfcfc] px-3 text-center text-3xl uppercase text-black outline-none"
            aria-label="Manual stand"
            value={manualStand}
            maxLength={8}
            onChange={e => setManualStand(e.target.value.toUpperCase())}
            onKeyDown={e => { if (e.key === "Enter") send(); if (e.key === "Escape") close(); }}
            autoFocus
          />
          <MenuButton onClick={close}>ESC</MenuButton>
        </div>
      </div>
    </div>
  );
}

function MenuButton({ children, onClick }: { children: ReactNode; onClick: () => void }) {
  return <button className="h-[4.4rem] bg-[#3f3f3f] px-5 text-2xl font-semibold text-white shadow" onClick={onClick}>{children}</button>;
}

function LegacyArrStandDialog({ open, onOpenChange, callsign, currentStand }: Props) {
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
                  startReqActive={false}
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
