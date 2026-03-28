import { useEffect, useMemo, useReducer, useRef, useState } from "react";

import { Bay, type FrontendStrip } from "@/api/models";
import FlightPlanDialog from "@/components/FlightPlanDialog";
import EsetDeIceDialog from "@/components/eset/EsetDeIceDialog";
import EsetStandCell from "@/components/eset/EsetStandCell";
import EsetStandMenu, { type EsetMenuAnchor } from "@/components/eset/EsetStandMenu";
import EsetStandStatusDialog from "@/components/eset/EsetStandStatusDialog";
import {
  ESET_BACKGROUND_BOXES,
  ESET_BOARD_HEIGHT,
  ESET_BOARD_WIDTH,
  ESET_STANDS,
  parseTimestampMs,
} from "@/components/eset/metadata";
import { useNonClearedStrips } from "@/store/airports/ekch.ts";
import { useWebSocketStore } from "@/store/store-hooks.ts";

const PAGE_BG           = "bg-bay-eset";  // ESET uses a lighter panel grey than other views
const COLOR_LABEL_DEFAULT = "#202020";       // default label color for ESET background boxes

type ActionOverride = {
  callsign: string;
  blinking: boolean;
};

type ActionOverrideMap = Record<string, ActionOverride>;

type ActionOverrideAction =
  | { type: "set"; stand: string; override: ActionOverride }
  | { type: "prune"; occupancy: Record<string, string> };

interface CtotState {
  previous: Record<string, string>;
  flags: Record<string, boolean>;
}

function ctotReducer(state: CtotState, strips: FrontendStrip[]): CtotState {
  const nextPrevious: Record<string, string> = {};
  const nextFlags: Record<string, boolean> = {};

  for (const strip of strips) {
    const previousCtot = state.previous[strip.callsign] ?? "";
    const previousFlag = state.flags[strip.callsign] ?? false;

    nextPrevious[strip.callsign] = strip.ctot;

    if (!strip.ctot) {
      nextFlags[strip.callsign] = false;
      continue;
    }

    if (previousCtot && strip.ctot !== previousCtot) {
      const currentMs = parseTimestampMs(strip.ctot);
      const previousMs = parseTimestampMs(previousCtot);
      nextFlags[strip.callsign] =
        currentMs !== null && previousMs !== null ? currentMs < previousMs : false;
      continue;
    }

    nextFlags[strip.callsign] = previousFlag;
  }

  return {
    previous: nextPrevious,
    flags: nextFlags,
  };
}

function actionOverrideReducer(state: ActionOverrideMap, action: ActionOverrideAction): ActionOverrideMap {
  if (action.type === "set") {
    return {
      ...state,
      [action.stand]: action.override,
    };
  }

  const next = { ...state };

  for (const [stand, override] of Object.entries(state)) {
    if (action.occupancy[stand] !== override.callsign) {
      delete next[stand];
    }
  }

  return next;
}

function toMenuAnchor(element: HTMLButtonElement): EsetMenuAnchor {
  const rect = element.getBoundingClientRect();

  return {
    top: rect.top,
    left: rect.left,
    right: rect.right,
    bottom: rect.bottom,
  };
}

export default function ESET() {
  const strips = useWebSocketStore((state) => state.strips);
  const move = useWebSocketStore((state) => state.move);
  const updateStrip = useWebSocketStore((state) => state.updateStrip);
  const pickupStrip = useWebSocketStore((state) => state.pickupStrip);
  const transferStrip = useWebSocketStore((state) => state.transferStrip);
  const toggleMarked = useWebSocketStore((state) => state.toggleMarked);
  const cdmReady = useWebSocketStore((state) => state.cdmReady);
  const nonClearedStrips = useNonClearedStrips();

  const [menuState, setMenuState] = useState<{ stand: string; anchor: EsetMenuAnchor } | null>(null);
  const [statusStand, setStatusStand] = useState<string | null>(null);
  const [statusAnchor, setStatusAnchor] = useState<EsetMenuAnchor | null>(null);
  const [deIceOpen, setDeIceOpen] = useState(false);
  const [flightPlanCallsign, setFlightPlanCallsign] = useState<string | null>(null);
  const [blockedStands, setBlockedStands] = useState<Record<string, true>>({});
  const [nowMs, setNowMs] = useState(() => Date.now());
  const [boardScale, setBoardScale] = useState(1);
  const [ctotState, updateCtotState] = useReducer(ctotReducer, {
    previous: {},
    flags: {},
  });
  const [actionOverrides, updateActionOverrides] = useReducer(actionOverrideReducer, {});
  const boardFrameRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const intervalId = window.setInterval(() => setNowMs(Date.now()), 30_000);
    return () => window.clearInterval(intervalId);
  }, []);

  useEffect(() => {
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
  }, []);

  useEffect(() => {
    updateCtotState(strips);
  }, [strips]);

  const stripByStand = useMemo(() => {
    const mapping = new Map<string, FrontendStrip>();

    for (const strip of strips) {
      if (!strip.stand || strip.bay === Bay.Hidden) {
        continue;
      }

      mapping.set(strip.stand, strip);
    }

    return mapping;
  }, [strips]);

  const standOccupancy = useMemo(
    () => Object.fromEntries([...stripByStand.entries()].map(([stand, strip]) => [stand, strip.callsign])),
    [stripByStand],
  );

  useEffect(() => {
    updateActionOverrides({ type: "prune", occupancy: standOccupancy });
  }, [standOccupancy]);

  const menuStrip = menuState ? stripByStand.get(menuState.stand) : undefined;
  const statusStrip = statusStand ? stripByStand.get(statusStand) : undefined;

  function closeMenu() {
    setMenuState(null);
    setDeIceOpen(false);
  }

  function closeAllOverlays() {
    setMenuState(null);
    setStatusStand(null);
    setStatusAnchor(null);
    setDeIceOpen(false);
    setFlightPlanCallsign(null);
  }

  function setActionState(stand: string, strip: FrontendStrip, blinking = false) {
    updateActionOverrides({
      type: "set",
      stand,
      override: {
        callsign: strip.callsign,
        blinking,
      },
    });
  }

  function clearBlockedStand(stand: string) {
    setBlockedStands((current) => {
      const next = { ...current };
      delete next[stand];
      return next;
    });
  }

  function handleStandClick(stand: string, strip: FrontendStrip | undefined, element: HTMLButtonElement) {
    const blocked = !!blockedStands[stand];

    if (!strip || blocked) {
      setStatusStand(stand);
      setStatusAnchor(toMenuAnchor(element));
      setMenuState(null);
      return;
    }

    setMenuState({ stand, anchor: toMenuAnchor(element) });
    setStatusStand(null);
    setStatusAnchor(null);
  }

  function handleSendReady() {
    if (!menuStrip || !menuState) {
      return;
    }

    cdmReady(menuStrip.callsign);
    closeMenu();
  }

  function handleStartTransfer() {
    if (!menuStrip || !menuState) {
      return;
    }

    setActionState(menuState.stand, menuStrip);
    transferStrip(menuStrip.callsign, "AD");
    closeMenu();
  }

  function handleStartRequest() {
    if (!menuStrip || !menuState) {
      return;
    }

    setActionState(menuState.stand, menuStrip, true);
    closeMenu();
  }

  function handlePush() {
    if (!menuStrip || !menuState) {
      return;
    }

    setActionState(menuState.stand, menuStrip);
    pickupStrip(menuStrip.callsign, Bay.Push);
    closeMenu();
  }

  function handleTaxi() {
    if (!menuStrip || !menuState) {
      return;
    }

    setActionState(menuState.stand, menuStrip);
    pickupStrip(menuStrip.callsign, Bay.Taxi);
    closeMenu();
  }

  function handleToggleMarked() {
    if (!menuStrip) {
      return;
    }

    toggleMarked(menuStrip.callsign, !menuStrip.marked);
    closeMenu();
  }

  function handleOpenFlightPlan() {
    if (!menuStrip) {
      return;
    }

    setFlightPlanCallsign(menuStrip.callsign);
    setMenuState(null);
  }

  function handleStandOccupied() {
    if (!statusStand) {
      return;
    }

    // TODO: send stand_occupied action to backend
    setBlockedStands((current) => ({ ...current, [statusStand]: true }));
    setStatusStand(null);
    setMenuState(null);
  }

  function handleStandVacant() {
    if (!statusStand) {
      return;
    }

    clearBlockedStand(statusStand);
    setStatusStand(null);
    setMenuState(null);
  }

  function handleClearFpl() {
    if (!statusStand || !statusStrip) {
      return;
    }

    move(statusStrip.callsign, Bay.Hidden);
    setStatusStand(null);
    setMenuState(null);
  }

  function handleAssignPlannedDeparture(strip: FrontendStrip) {
    if (!statusStand) {
      return;
    }

    clearBlockedStand(statusStand);
    updateStrip(strip.callsign, { stand: statusStand });
    setStatusStand(null);
    setMenuState(null);
  }

  function handleSelectDeIcePlatform(platform: string) {
    if (!menuStrip) {
      return;
    }

    // TODO: send de-ice platform assignment to backend
    console.log("Selected de-ice platform", { platform, callsign: menuStrip.callsign });
    closeAllOverlays();
  }

  return (
    <div className={`h-[95.28vh] w-full overflow-hidden ${PAGE_BG} px-2 py-1`}>
      <div ref={boardFrameRef} className="relative h-full w-full overflow-hidden">
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
                key={`${box.x}-${box.y}-${box.width}-${box.height}`}
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

            {ESET_STANDS.map((stand) => {
              const strip = stripByStand.get(stand.label);
              const actionOverride = strip ? actionOverrides[stand.label] : undefined;
              const actionActive = !!actionOverride && !!strip && actionOverride.callsign === strip.callsign;

              return (
                <EsetStandCell
                  key={stand.label}
                  stand={stand}
                  strip={strip}
                  blocked={!!blockedStands[stand.label]}
                  actionActive={actionActive}
                  blinking={actionActive ? actionOverride.blinking : false}
                  ctotImproved={strip ? !!ctotState.flags[strip.callsign] : false}
                  nowMs={nowMs}
                  containerStyle={{
                    position: "absolute",
                    left: stand.left,
                    top: stand.top,
                  }}
                  onClick={handleStandClick}
                />
              );
            })}
          </div>
        </div>
      </div>

      {menuState && menuStrip && (
        <EsetStandMenu
          open
          anchor={menuState.anchor}
          strip={menuStrip}
          onClose={closeMenu}
          onSendReady={handleSendReady}
          onStartTransfer={handleStartTransfer}
          onStartRequest={handleStartRequest}
          onPush={handlePush}
          onTaxi={handleTaxi}
          onOpenDeIce={() => setDeIceOpen(true)}
          onOpenFlightPlan={handleOpenFlightPlan}
          onToggleMarked={handleToggleMarked}
          onOpenStandStatus={() => {
            setStatusStand(menuState.stand);
            setStatusAnchor(menuState.anchor);
          }}
        />
      )}

      {statusStand && (
        <EsetStandStatusDialog
          key={statusStand}
          open
          stand={statusStand}
          anchor={statusAnchor}
          strip={statusStrip}
          nonClearedStrips={nonClearedStrips}
          onClose={() => {
            setStatusStand(null);
            setStatusAnchor(null);
          }}
          onOccupied={handleStandOccupied}
          onVacant={handleStandVacant}
          onClearFpl={handleClearFpl}
          onAssignPlannedDeparture={handleAssignPlannedDeparture}
        />
      )}

      <EsetDeIceDialog
        open={deIceOpen}
        strip={menuStrip}
        onOpenChange={setDeIceOpen}
        onSelectPlatform={handleSelectDeIcePlatform}
      />

      {flightPlanCallsign && (
        <FlightPlanDialog
          callsign={flightPlanCallsign}
          open={!!flightPlanCallsign}
          onOpenChange={(open) => {
            if (!open) {
              setFlightPlanCallsign(null);
            }
          }}
        />
      )}
    </div>
  );
}
