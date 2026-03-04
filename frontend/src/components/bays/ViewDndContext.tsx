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
import type { Bay, StripRef } from "@/api/models.ts";
import { useState, type ReactNode } from "react";
import { DragStateContext } from "./DragStateContext";
import { DragDisabledContext } from "./DragDisabledContext";

export interface BayConfig {
  strips: { callsign: string; sequence?: number }[];
  /** Backend Bay enum value this visual bay maps to */
  targetBay: Bay;
  /** When true, strips are displayed highest-sequence-first (descending). Affects cross-bay reorder fallback. */
  descending?: boolean;
}

interface ViewDndContextProps {
  children: ReactNode;
  /** Maps visual bay ID -> strip list + backend Bay. Used to resolve drag source/target. */
  bayStripMap: Record<string, BayConfig>;
  /** Maps source bay ID -> list of bay IDs a strip may be dragged into */
  transferRules: Record<string, string[]>;
  onReorder: (callsign: string, above: StripRef | null) => void;
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
  const [dragDisabled, setDragDisabled] = useState(false);
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: dragDisabled ? Infinity : 5 } })
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

    // insert_after = the strip that should be the predecessor (insert_after semantics)
    const targetStrips = bayStripMap[targetBayId].strips;
    const targetDescending = bayStripMap[targetBayId].descending ?? false;

    if (sourceBayId === targetBayId) {
      // overId may be a bay container ID (not a strip callsign) when the cursor
      // hovers over the empty area of the bay. Handle this case explicitly.
      const overIsBayContainer = !(targetStrips.some(s => s.callsign === overId));
      const activeSeq = targetStrips.find(s => s.callsign === callsign)?.sequence;
      const overSeq = targetStrips.find(s => s.callsign === overId)?.sequence;
      let insertAfter: StripRef | null;
      if (overIsBayContainer) {
        // Cursor is over the bay container (not a strip). For descending bays the
        // container is at the visual bottom (below all strips), so insert after the
        // current highest-seq strip to become the new visual top. For ascending bays
        // the container is at the top, so null = new visual top.
        if (targetDescending) {
          const topStrip = targetStrips
            .filter(s => s.callsign !== callsign && s.sequence !== undefined)
            .sort((a, b) => (b.sequence ?? 0) - (a.sequence ?? 0))[0];
          insertAfter = topStrip ? { kind: "flight", callsign: topStrip.callsign } : null;
        } else {
          insertAfter = null;
        }
      } else if (activeSeq !== undefined && overSeq !== undefined) {
        if (activeSeq < overSeq) {
          // Moving toward higher seq: over strip becomes the predecessor
          insertAfter = { kind: "flight", callsign: overId };
        } else {
          // Moving toward lower seq: find the strip with the highest seq below overSeq
          const prevStrip = targetStrips
            .filter(s => s.callsign !== callsign && s.sequence !== undefined && s.sequence < overSeq)
            .sort((a, b) => (b.sequence ?? 0) - (a.sequence ?? 0))[0];
          insertAfter = prevStrip ? { kind: "flight", callsign: prevStrip.callsign } : null;
        }
      } else {
        // Fallback: index comparison (ascending-sorted arrays only)
        const activeIndex = targetStrips.findIndex(s => s.callsign === callsign);
        const overIndex = targetStrips.findIndex(s => s.callsign === overId);
        if (activeIndex < overIndex) {
          insertAfter = { kind: "flight", callsign: overId };
        } else {
          const prevCallsign = targetStrips[overIndex - 1]?.callsign ?? null;
          insertAfter = prevCallsign ? { kind: "flight", callsign: prevCallsign } : null;
        }
      }
      onReorder(callsign, insertAfter);
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
    // Same logical bay, different visual bay: use sequence-aware insertion.
    // For descending bays, dragging "above" the bay hits another bay's strip (low seq) — instead
    // insert after the current top strip (highest seq) to go to the visual top.
    const sourceStrips = bayStripMap[sourceBayId].strips;
    const sourceDescending = bayStripMap[sourceBayId].descending ?? false;
    let crossInsertAfter: StripRef | null;
    if (sourceDescending) {
      // Find the highest-seq strip in the source bay (excluding the active strip) → that becomes the predecessor for visual top
      const topStrip = sourceStrips
        .filter(s => s.callsign !== callsign && s.sequence !== undefined)
        .sort((a, b) => (b.sequence ?? 0) - (a.sequence ?? 0))[0];
      crossInsertAfter = topStrip ? { kind: "flight", callsign: topStrip.callsign } : null;
    } else {
      crossInsertAfter = targetStrips.some(s => s.callsign === overId) ? { kind: "flight", callsign: overId } : null;
    }
    onReorder(callsign, crossInsertAfter);
  }

  return (
    <DragDisabledContext value={{ setDragDisabled }}>
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
        <DragStateContext value={{ activeId, isValidTarget }}>
          {children}
          {renderDragOverlay && (
            <DragOverlay style={{ opacity: 0.5 }}>
              {activeId ? renderDragOverlay(activeId) : null}
            </DragOverlay>
          )}
        </DragStateContext>
      </DndContext>
    </DragDisabledContext>
  );
}
