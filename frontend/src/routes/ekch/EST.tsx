import { useEffect, useMemo, useReducer, useRef, useState } from "react";

import { Bay, type FrontendStrip } from "@/api/models";
import FlightPlanDialog from "@/components/FlightPlanDialog";
import EstDeIceDialog from "@/components/est/EstDeIceDialog";
import EstStandCell from "@/components/est/EstStandCell";
import EstStandMenu, { type EstMenuAnchor } from "@/components/est/EstStandMenu";
import EstStandStatusDialog from "@/components/est/EstStandStatusDialog";
import EstViewButtons from "@/components/est/EstViewButtons";
import { isEstDepartureTransferActive } from "@/components/est/transferState";
import { isTsatWithinStartRequestWindow } from "@/lib/cdmColors";
import { deriveEstStandBlocking } from "@/components/est/standBlocking";
import { deriveEstStandDisplay } from "@/components/est/standDisplay";
import {
  EST_BACKGROUND_BOXES,
  EST_BOARD_HEIGHT,
  EST_BOARD_WIDTH,
  getEstStandsForView,
  parseTimestampMs,
  type EstView,
} from "@/components/est/metadata";
import { useNonClearedStrips } from "@/store/airports/ekch.ts";
import { useControllers, useMarkArmed, useMyPosition, useSelectStrip, useSelectedCallsign, useWebSocketStore, useSatEnabled, useStandAssignments, useStandBlocks, useOccupyStand, useVacateStand, useRequestManualStand, useStripTransfers } from "@/store/store-hooks.ts";

const PAGE_BG = "bg-bay-est";
const COLOR_LABEL_DEFAULT = "#202020";
const APRON_DEPARTURE_SECTOR = "AD";

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

function toMenuAnchor(element: HTMLButtonElement): EstMenuAnchor {
  const rect = element.getBoundingClientRect();

  return {
    top: rect.top,
    left: rect.left,
    right: rect.right,
    bottom: rect.bottom,
  };
}

export default function EST() {
  const strips = useWebSocketStore((state) => state.strips);
  const move = useWebSocketStore((state) => state.move);
  const updateStrip = useWebSocketStore((state) => state.updateStrip);
  const pickupStrip = useWebSocketStore((state) => state.pickupStrip);
  const transferStrip = useWebSocketStore((state) => state.transferStrip);
  const setStartReq = useWebSocketStore((state) => state.setStartReq);
  const toggleMarked = useWebSocketStore((state) => state.toggleMarked);
  const controllers = useControllers();
  const myPosition = useMyPosition();
  const nonClearedStrips = useNonClearedStrips();
  const markArmed = useMarkArmed();
  const selectedCallsign = useSelectedCallsign();
  const selectStrip = useSelectStrip();
  const satEnabled = useSatEnabled();
  const standBlocks = useStandBlocks();
  const standAssignments = useStandAssignments();
  const occupyStand = useOccupyStand();
  const vacateStand = useVacateStand();
  const requestManualStand = useRequestManualStand();
  const stripTransfers = useStripTransfers();

  const [menuState, setMenuState] = useState<{ stand: string; anchor: EstMenuAnchor } | null>(null);
  const [statusStand, setStatusStand] = useState<string | null>(null);
  const [statusAnchor, setStatusAnchor] = useState<EstMenuAnchor | null>(null);
  const [deIceOpen, setDeIceOpen] = useState(false);
  const [deIcePlatforms, setDeIcePlatforms] = useState<Record<string, string>>({});
  const [flightPlanCallsign, setFlightPlanCallsign] = useState<string | null>(null);
  const [blockedStands, setBlockedStands] = useState<Record<string, true>>({});
  const [nowMs, setNowMs] = useState(() => Date.now());
  const [boardScale, setBoardScale] = useState(1);
  const [boardView, setBoardView] = useState<EstView>("MAIN");
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
  }, []);

  useEffect(() => {
    updateCtotState(strips);
  }, [strips]);

  const standDisplay = useMemo(
    () => deriveEstStandDisplay(strips, standAssignments, satEnabled),
    [satEnabled, standAssignments, strips],
  );
  const assignmentByStand = standDisplay.assignmentsByStand;
  const stripByStand = standDisplay.stripsByStand;

  const standOccupancy = useMemo(
    () => Object.fromEntries([...stripByStand.entries()].map(([stand, strip]) => [stand, strip.callsign])),
    [stripByStand],
  );

  const satStandBlocking = useMemo(
    () => deriveEstStandBlocking(standAssignments, standBlocks),
    [standAssignments, standBlocks],
  );
  const blockedStandsDerived = satEnabled ? satStandBlocking.blocked : blockedStands;
  const standBlockReasons = satEnabled ? satStandBlocking.reasons : {};

  useEffect(() => {
    updateActionOverrides({ type: "prune", occupancy: standOccupancy });
  }, [standOccupancy]);

  const menuStrip = menuState ? stripByStand.get(menuState.stand) : undefined;
  const statusStrip = statusStand ? stripByStand.get(statusStand) : undefined;
  const visibleStands = useMemo(() => getEstStandsForView(boardView), [boardView]);
  const apronDepartureTransferTarget = useMemo(
    () =>
      controllers.find(
        (controller) =>
          controller.position !== myPosition &&
          controller.owned_sectors.includes(APRON_DEPARTURE_SECTOR),
      )?.position ?? "",
    [controllers, myPosition],
  );

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

  function handleBoardViewChange(nextView: EstView) {
    closeAllOverlays();
    setBoardView(nextView);
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
    if (satEnabled) {
      const block = standBlocks.find((candidate) =>
        candidate.stand === stand &&
        candidate.block_type === "MANUAL" &&
        candidate.created_by?.toUpperCase() === myPosition.toUpperCase() &&
        candidate.id !== undefined &&
        candidate.version !== undefined
      );
      if (block?.id !== undefined && block.version !== undefined) vacateStand(stand, block.id, block.version);
    } else {
      setBlockedStands((current) => {
        const next = { ...current };
        delete next[stand];
        return next;
      });
    }
  }

  function handleStandClick(stand: string, strip: FrontendStrip | undefined, element: HTMLButtonElement) {
    const blocked = !!blockedStandsDerived[stand];

    if (!strip || blocked) {
      setStatusStand(stand);
      setStatusAnchor(toMenuAnchor(element));
      setMenuState(null);
      return;
    }

    if (markArmed) {
      toggleMarked(strip.callsign, !strip.marked);
      setMenuState(null);
      setStatusStand(null);
      setStatusAnchor(null);
      setDeIceOpen(false);
      return;
    }

    if (selectedCallsign !== strip.callsign) {
      selectStrip(strip.callsign);
    }

    setMenuState({ stand, anchor: toMenuAnchor(element) });
    setStatusStand(null);
    setStatusAnchor(null);
    setDeIceOpen(false);
  }

  function handleStartTransfer() {
    if (!menuStrip || !menuState) {
      return;
    }

    if (!apronDepartureTransferTarget) {
      return;
    }

    setStartReq(menuStrip.callsign, true);
    setActionState(menuState.stand, menuStrip);
    transferStrip(menuStrip.callsign, apronDepartureTransferTarget);
    closeMenu();
  }

  function handleStartRequest() {
    if (!menuStrip || !menuState) {
      return;
    }

    setStartReq(menuStrip.callsign, !menuStrip.start_req);
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

    if (satEnabled) {
      occupyStand(statusStand);
    } else {
      setBlockedStands((current) => ({ ...current, [statusStand]: true }));
    }
    setStatusStand(null);
    setMenuState(null);
  }

  function handleStandVacant() {
    if (!statusStand) {
      return;
    }

    if (satEnabled) {
      const block = standBlocks.find((candidate) =>
        candidate.stand === statusStand &&
        candidate.block_type === "MANUAL" &&
        candidate.created_by?.toUpperCase() === myPosition.toUpperCase() &&
        candidate.id !== undefined &&
        candidate.version !== undefined
      );
      if (block?.id !== undefined && block.version !== undefined) vacateStand(statusStand, block.id, block.version);
    } else {
      clearBlockedStand(statusStand);
    }
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

    if (satEnabled) {
      const version = standAssignments.find((assignment) => assignment.callsign === strip.callsign)?.version ?? 0;
      requestManualStand(strip.callsign, statusStand, version);
    } else {
      clearBlockedStand(statusStand);
      updateStrip(strip.callsign, { stand: statusStand });
    }
    setStatusStand(null);
    setMenuState(null);
  }

  function handleSelectDeIcePlatform(platform: string) {
    if (!menuStrip) {
      return;
    }

    setDeIcePlatforms((current) => ({
      ...current,
      [menuStrip.callsign]: platform,
    }));
    closeAllOverlays();
  }

  function handleEraseDeIcePlatform() {
    if (!menuStrip) {
      return;
    }

    setDeIcePlatforms((current) => {
      const next = { ...current };
      delete next[menuStrip.callsign];
      return next;
    });
    closeAllOverlays();
  }

  return (
    <div className={`h-[95.28dvh] w-full overflow-hidden ${PAGE_BG} px-2 py-1`}>
      <div ref={boardFrameRef} className="relative h-full w-full overflow-hidden">
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

            <EstViewButtons view={boardView} onViewChange={handleBoardViewChange} />

            {visibleStands.map((stand) => {
              const strip = stripByStand.get(stand.label);
              const assignment = assignmentByStand.get(stand.label);
              const actionOverride = strip ? actionOverrides[stand.label] : undefined;
              const actionActive = !!actionOverride && !!strip && actionOverride.callsign === strip.callsign;
              const startReqActive = !!strip?.start_req;

              return (
                <EstStandCell
                  key={stand.label}
                  stand={stand}
                  strip={strip}
                  assignment={assignment}
                  selected={!!strip && selectedCallsign === strip.callsign}
                  blocked={!!blockedStandsDerived[stand.label]}
                  blockReason={standBlockReasons[stand.label]}
                  actionActive={actionActive}
                  blinking={actionActive ? actionOverride.blinking : false}
                  startReqActive={startReqActive}
                  departureTransferActive={isEstDepartureTransferActive(
                    strip ? stripTransfers[strip.callsign] : undefined,
                    myPosition,
                    apronDepartureTransferTarget,
                  )}
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
        <EstStandMenu
          open
          anchor={menuState.anchor}
          strip={menuStrip}
          onClose={closeMenu}
          onStartTransfer={handleStartTransfer}
          startTransferDisabled={
            !apronDepartureTransferTarget ||
            !isTsatWithinStartRequestWindow(menuStrip.tsat, nowMs)
          }
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
        <EstStandStatusDialog
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

      <EstDeIceDialog
        open={deIceOpen}
        strip={menuStrip}
        selectedPlatform={menuStrip ? deIcePlatforms[menuStrip.callsign] : undefined}
        onOpenChange={setDeIceOpen}
        onSelectPlatform={handleSelectDeIcePlatform}
        onErase={handleEraseDeIcePlatform}
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
