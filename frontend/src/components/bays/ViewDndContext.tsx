import {
  DndContext,
  DragOverlay,
  closestCenter,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import type { Bay } from "@/api/models.ts";
import { createContext, useContext, useState, type ReactNode } from "react";

export interface BayConfig {
  strips: { callsign: string }[];
  /** Backend Bay enum value this visual bay maps to */
  targetBay: Bay;
}

interface DragState {
  /** Callsign of the strip currently being dragged, or null. */
  activeId: string | null;
  /** Returns true if the active strip may be dropped into the given bay. */
  isValidTarget: (bayId: string) => boolean;
}

const DragStateContext = createContext<DragState>({ activeId: null, isValidTarget: () => true });

/** Consume drag state (activeId + validity check) inside any bay component. */
export function useDragState() {
  return useContext(DragStateContext);
}

interface ViewDndContextProps {
  children: ReactNode;
  /** Maps visual bay ID -> strip list + backend Bay. Used to resolve drag source/target. */
  bayStripMap: Record<string, BayConfig>;
  /** Maps source bay ID -> list of bay IDs a strip may be dragged into */
  transferRules: Record<string, string[]>;
  onReorder: (callsign: string, before: string | null) => void;
  onMove: (callsign: string, bay: Bay) => void;
  /** Renders the floating drag preview that follows the cursor across bay boundaries. */
  renderDragOverlay?: (callsign: string) => ReactNode;
}

export function ViewDndContext({
  children,
  bayStripMap,
  transferRules,
  onReorder,
  onMove,
  renderDragOverlay,
}: ViewDndContextProps) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } })
  );

  const [activeId, setActiveId] = useState<string | null>(null);

  /** Returns the visual bay ID that "owns" the given drag item or container ID. */
  function findBayId(id: string): string | null {
    if (id in bayStripMap) return id;
    for (const [bayId, { strips }] of Object.entries(bayStripMap)) {
      if (strips.some(s => s.callsign === id)) return bayId;
    }
    return null;
  }

  /** True when the active strip is allowed to be dropped into the given bay. */
  function isValidTarget(targetBayId: string): boolean {
    if (!activeId) return true;
    const sourceBayId = findBayId(activeId);
    if (!sourceBayId) return true;
    if (sourceBayId === targetBayId) return true; // reorder within same bay is always ok
    return (transferRules[sourceBayId] ?? []).includes(targetBayId);
  }

  function handleDragStart(event: DragStartEvent) {
    setActiveId(event.active.id as string);
  }

  function handleDragEnd(event: DragEndEvent) {
    setActiveId(null);
    const { active, over } = event;
    if (!over) return;

    const callsign = active.id as string;
    const overId = over.id as string;
    if (callsign === overId) return;

    const sourceBayId = findBayId(callsign);
    const targetBayId = findBayId(overId);
    if (!sourceBayId || !targetBayId) return;

    // `before` = callsign to insert before, or null to append
    const targetStrips = bayStripMap[targetBayId].strips;
    const before = targetStrips.some(s => s.callsign === overId) ? overId : null;

    if (sourceBayId === targetBayId) {
      onReorder(callsign, before);
      return;
    }

    // Cross-bay: enforce transfer rules
    const allowed = transferRules[sourceBayId] ?? [];
    if (!allowed.includes(targetBayId)) return;

    const sourceBay = bayStripMap[sourceBayId].targetBay;
    const targetBay = bayStripMap[targetBayId].targetBay;

    // Only emit a move event when the backend bay actually changes.
    // Do NOT also call onReorder here: the backend's FrontendBay response to
    // FrontendUpdateOrder carries the full strip state (including bay). If the
    // move hasn't been committed to the DB yet when the order event is processed,
    // the response echoes the old bay back, reverting the optimistic move and
    // causing the strip to disappear. Send only FrontendMove; the backend assigns
    // the sequence as part of the move operation.
    if (sourceBay !== targetBay) {
      onMove(callsign, targetBay);
      return;
    }
    onReorder(callsign, before);
  }

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
      <DragStateContext value={{ activeId, isValidTarget }}>
        {children}
        {renderDragOverlay && (
          <DragOverlay>
            {activeId ? renderDragOverlay(activeId) : null}
          </DragOverlay>
        )}
      </DragStateContext>
    </DndContext>
  );
}
