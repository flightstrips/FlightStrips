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
import type { AnyStrip, Bay, StripRef } from "@/api/models.ts";
import { stripDndId } from "@/api/models.ts";
import { useState, type ReactNode } from "react";

/** Builds a StripRef from a DnD item id, which may be a flight callsign or "tactical-<N>". */
function makeStripRef(id: string | null | undefined): StripRef | null {
  if (!id) return null;
  if (id.startsWith("tactical-")) {
    return { kind: "tactical", id: parseInt(id.slice("tactical-".length), 10) };
  }
  return { kind: "flight", callsign: id };
}
import { DragStateContext } from "./DragStateContext";
import { DragDisabledContext } from "./DragDisabledContext";
import { BayClickContext } from "./BayClickContext";
import { useSelectedCallsign, useSelectStrip } from "@/store/store-hooks";

export interface BayConfig {
  strips: AnyStrip[];
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
  onReorder: (activeRef: StripRef, above: StripRef | null) => void;
  onMove: (callsign: string, bay: Bay) => void;
  /** Renders the floating drag preview that follows the cursor across bay boundaries. */
  renderDragOverlay?: (strip: AnyStrip) => ReactNode;
}

export function ViewDndContext({
  children,
  bayStripMap,
  transferRules,
  onReorder,
  onMove,
  renderDragOverlay,
}: ViewDndContextProps) {
  const selectedCallsign = useSelectedCallsign();
  const selectStrip = useSelectStrip();

  const [dragDisabled, setDragDisabled] = useState(false);
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: dragDisabled ? Infinity : 5 } })
  );

  const [activeId, setActiveId] = useState<string | null>(null);

  /** Returns the visual bay ID that "owns" the given drag item or container ID. */
  function findBayId(id: string): string | null {
    if (id in bayStripMap) return id;
    for (const [bayId, { strips }] of Object.entries(bayStripMap)) {
      if (strips.some(s => stripDndId(s) === id)) return bayId;
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

  function handleBayClick(clickedBayId: string) {
    if (!selectedCallsign) return;

    const sourceBayId = findBayId(selectedCallsign);
    if (!sourceBayId) return;
    if (sourceBayId === clickedBayId) return;

    const allowed = transferRules[sourceBayId] ?? [];
    if (!allowed.includes(clickedBayId)) return;

    const targetConfig = bayStripMap[clickedBayId];
    if (!targetConfig) return;

    onMove(selectedCallsign, targetConfig.targetBay);
    selectStrip(null);
  }

  function handleDragStart(event: DragStartEvent) {
    setActiveId(event.active.id as string);
  }

  function handleDragEnd(event: DragEndEvent) {
    setActiveId(null);
    const { active, over } = event;
    if (!over) return;

    const dndId = active.id as string;
    const overId = over.id as string;
    if (dndId === overId) return;

    const sourceBayId = findBayId(dndId);
    const targetBayId = findBayId(overId);
    if (!sourceBayId || !targetBayId) return;

    // insert_after = the strip that should be the predecessor (insert_after semantics)
    const targetStrips = bayStripMap[targetBayId].strips;
    const targetDescending = bayStripMap[targetBayId].descending ?? false;

    if (sourceBayId === targetBayId) {
      // overId may be a bay container ID (not a strip dnd id) when the cursor
      // hovers over the empty area of the bay. Handle this case explicitly.
      const overIsBayContainer = !(targetStrips.some(s => stripDndId(s) === overId));
      const activeSeq = targetStrips.find(s => stripDndId(s) === dndId)?.sequence;
      const overSeq = targetStrips.find(s => stripDndId(s) === overId)?.sequence;
      let insertAfter: StripRef | null;
      if (overIsBayContainer) {
        // Cursor is over the bay container (not a strip). For descending bays the
        // container is at the visual bottom (below all strips), so insert after the
        // current highest-seq strip to become the new visual top. For ascending bays
        // the container is at the top, so null = new visual top.
        if (targetDescending) {
          const topStrip = targetStrips
            .filter(s => stripDndId(s) !== dndId && s.sequence !== undefined)
            .sort((a, b) => (b.sequence ?? 0) - (a.sequence ?? 0))[0];
          insertAfter = topStrip ? makeStripRef(stripDndId(topStrip)) : null;
        } else {
          insertAfter = null;
        }
      } else if (activeSeq !== undefined && overSeq !== undefined) {
        if (activeSeq < overSeq) {
          // Moving toward higher seq: over strip becomes the predecessor
          insertAfter = makeStripRef(overId);
        } else {
          // Moving toward lower seq: find the strip with the highest seq below overSeq
          const prevStrip = targetStrips
            .filter(s => stripDndId(s) !== dndId && s.sequence !== undefined && s.sequence < overSeq)
            .sort((a, b) => (b.sequence ?? 0) - (a.sequence ?? 0))[0];
          insertAfter = prevStrip ? makeStripRef(stripDndId(prevStrip)) : null;
        }
      } else {
        // Fallback: index comparison (ascending-sorted arrays only)
        const activeIndex = targetStrips.findIndex(s => stripDndId(s) === dndId);
        const overIndex = targetStrips.findIndex(s => stripDndId(s) === overId);
        if (activeIndex < overIndex) {
          insertAfter = makeStripRef(overId);
        } else {
          const prevStrip = targetStrips[overIndex - 1];
          insertAfter = prevStrip ? makeStripRef(stripDndId(prevStrip)) : null;
        }
      }
      onReorder(makeStripRef(dndId)!, insertAfter);
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
      // Only flight strips may cross bays
      if (!dndId.startsWith("tactical-")) {
        onMove(dndId, targetBay);
      }
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
        .filter(s => stripDndId(s) !== dndId && s.sequence !== undefined)
        .sort((a, b) => (b.sequence ?? 0) - (a.sequence ?? 0))[0];
      crossInsertAfter = topStrip ? makeStripRef(stripDndId(topStrip)) : null;
    } else {
      crossInsertAfter = targetStrips.some(s => stripDndId(s) === overId) ? makeStripRef(overId) : null;
    }
    onReorder(makeStripRef(dndId)!, crossInsertAfter);
  }

  // Resolve activeId string → AnyStrip for the drag overlay callback
  function resolveActiveStrip(): AnyStrip | null {
    if (!activeId) return null;
    for (const { strips } of Object.values(bayStripMap)) {
      const found = strips.find(s => stripDndId(s) === activeId);
      if (found) return found;
    }
    return null;
  }

  return (
    <BayClickContext value={{ onBayClick: handleBayClick }}>
      <DragDisabledContext value={{ setDragDisabled }}>
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
          <DragStateContext value={{ activeId, isValidTarget }}>
            {children}
            {renderDragOverlay && (
              <DragOverlay style={{ opacity: 0.5 }}>
                {activeId ? (() => {
                  const strip = resolveActiveStrip();
                  return strip ? renderDragOverlay(strip) : null;
                })() : null}
              </DragOverlay>
            )}
          </DragStateContext>
        </DndContext>
      </DragDisabledContext>
    </BayClickContext>
  );
}
