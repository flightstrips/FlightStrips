import {
  DndContext,
  closestCenter,
  useDroppable,
  useDndMonitor,
  type DragEndEvent,
  type DragOverEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  verticalListSortingStrategy,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useDragState } from "@/components/bays/DragStateContext";
import { useBayClick } from "./BayClickContext";
import { Children, useCallback, useLayoutEffect, useRef, useState, type CSSProperties, type ReactNode } from "react";
import type { AnyStrip, StripRef } from "@/api/models.ts";
import { stripDndId, isFlight } from "@/api/models.ts";
import { isValidationActiveForPosition } from "@/components/strip/shared";
import { useWebSocketStore } from "@/store/store-hooks";
import { useStripSensors } from "./useStripSensors";

interface SortableBayProps {
  strips: AnyStrip[];
  /** Required when standalone=true (default). Not used when standalone=false. */
  onReorder?: (callsign: string, insertAfter: StripRef | null) => void;
  children: (strip: AnyStrip) => ReactNode;
  className?: string;
  /** Unique bay identifier. Required when standalone=false so the container is droppable. */
  bayId?: string;
  /**
   * When true (default) wraps strips in its own DndContext for self-contained reordering.
   * When false, relies on a parent ViewDndContext; bayId must be provided.
   */
  standalone?: boolean;
  /** Optional predicate; when it returns true the strip's drag handle is disabled. */
  isDragDisabled?: (strip: AnyStrip) => boolean;
}

function useBottomAlignedBay(className?: string, dependencyKey?: string) {
  const ref = useRef<HTMLDivElement | null>(null);
  const frameRef = useRef<number | null>(null);
  const [shouldFill, setShouldFill] = useState(false);
  const isBottomAlignedBay = className?.includes("bay-scroll-area-bottom") || className?.includes("bay-scroll-area-dark");

  const measure = useCallback(() => {
    const node = ref.current;
    if (!node || !isBottomAlignedBay) {
      setShouldFill(false);
      return;
    }

    const computedStyle = getComputedStyle(node);
    const paddingTop = Number.parseFloat(computedStyle.paddingTop) || 0;
    const paddingBottom = Number.parseFloat(computedStyle.paddingBottom) || 0;
    const gap = Number.parseFloat(computedStyle.rowGap) || 0;
    const childCount = node.children.length;
    const childrenHeight = Array.from(node.children).reduce((sum, child) => {
      return sum + child.getBoundingClientRect().height;
    }, 0);
    const contentHeight = paddingTop + paddingBottom + childrenHeight + Math.max(0, childCount - 1) * gap;
    const fitsWithoutOverflow = contentHeight <= node.clientHeight + 0.5;

    setShouldFill(fitsWithoutOverflow);
    if (fitsWithoutOverflow && node.scrollTop !== 0) {
      node.scrollTop = 0;
    }
  }, [isBottomAlignedBay]);

  const scheduleMeasure = useCallback(() => {
    if (frameRef.current !== null) {
      cancelAnimationFrame(frameRef.current);
    }
    frameRef.current = requestAnimationFrame(() => {
      frameRef.current = null;
      measure();
    });
  }, [measure]);

  useLayoutEffect(() => {
    measure();
  }, [dependencyKey, measure]);

  useLayoutEffect(() => {
    const node = ref.current;
    if (!node || !isBottomAlignedBay) return;

    const resizeObserver = new ResizeObserver(() => scheduleMeasure());
    resizeObserver.observe(node);
    Array.from(node.children).forEach((child) => resizeObserver.observe(child));

    const mutationObserver = new MutationObserver(() => {
      resizeObserver.disconnect();
      resizeObserver.observe(node);
      Array.from(node.children).forEach((child) => resizeObserver.observe(child));
      scheduleMeasure();
    });
    mutationObserver.observe(node, { childList: true });

    const handleResize = () => scheduleMeasure();
    window.addEventListener("resize", handleResize);

    return () => {
      if (frameRef.current !== null) {
        cancelAnimationFrame(frameRef.current);
      }
      resizeObserver.disconnect();
      mutationObserver.disconnect();
      window.removeEventListener("resize", handleResize);
    };
  }, [dependencyKey, isBottomAlignedBay, scheduleMeasure]);

  return {
    containerRef(node: HTMLDivElement | null) {
      ref.current = node;
    },
    containerClassName: shouldFill ? `${className ?? ""} bay-scroll-fill` : className,
  };
}

export function AutoAlignedBay({
  className,
  children,
  dependencyKey,
}: {
  className?: string;
  children?: ReactNode;
  dependencyKey?: string;
}) {
  const { containerRef, containerClassName } = useBottomAlignedBay(className, dependencyKey);

  return (
    <div ref={containerRef} className={containerClassName}>
      {children}
    </div>
  );
}

export function SortableBay({
  strips,
  onReorder,
  children,
  className,
  bayId,
  standalone = true,
  isDragDisabled,
}: SortableBayProps) {
  const { containerRef, containerClassName } = useBottomAlignedBay(className, `${bayId ?? "standalone"}:${strips.length}`);
  const sensors = useStripSensors();

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event;
    if (!over || active.id === over.id || !onReorder) return;
    const activeIndex = strips.findIndex(s => stripDndId(s) === active.id);
    const overIndex = strips.findIndex(s => stripDndId(s) === over.id);
    let insertAfter: StripRef | null;
    if (activeIndex < overIndex) {
      // Dragging down: insert after over → insert_after = over (the predecessor)
      const overStrip = strips[overIndex];
      insertAfter = isFlight(overStrip)
        ? { kind: "flight", callsign: overStrip.callsign }
        : { kind: "tactical", id: overStrip.id };
    } else {
      // Dragging up: insert before over → insert_after = strip before over (or null = top)
      const prevStrip = strips[overIndex - 1];
      if (!prevStrip) {
        insertAfter = null;
      } else {
        insertAfter = isFlight(prevStrip)
          ? { kind: "flight", callsign: prevStrip.callsign }
          : { kind: "tactical", id: prevStrip.id };
      }
    }
    onReorder(active.id as string, insertAfter);
  }

  const sortableItems = strips.map(s => stripDndId(s));

  if (standalone) {
      return (
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
          <SortableContext items={sortableItems} strategy={verticalListSortingStrategy}>
          <div ref={containerRef} className={containerClassName} data-strip-scroll-container="true">
              {strips.map(s => (
                <SortableStrip key={stripDndId(s)} callsign={stripDndId(s)} dragDisabled={isDragDisabled?.(s)}>
                  {children(s)}
              </SortableStrip>
            ))}
          </div>
        </SortableContext>
      </DndContext>
    );
  }

  // Non-standalone: parent ViewDndContext owns the DndContext.
  // useDroppable makes empty bays valid drop targets.
  return (
    <SortableContext items={sortableItems} strategy={verticalListSortingStrategy}>
      <DroppableContainer bayId={bayId!} isEmpty={strips.length === 0} className={className}>
        {strips.map(s => (
          <SortableStrip key={stripDndId(s)} callsign={stripDndId(s)} hideWhenDragging bayId={bayId} dragDisabled={isDragDisabled?.(s)}>
            {children(s)}
          </SortableStrip>
        ))}
      </DroppableContainer>
    </SortableContext>
  );
}

function DroppableContainer({
  bayId,
  isEmpty,
  className,
  children,
}: {
  bayId: string;
  /** When false, do NOT register as a droppable — strips handle their own collision detection.
   *  Register only for empty bays so they remain valid cross-bay drop targets. */
  isEmpty: boolean;
  className?: string;
  children: ReactNode;
}) {
  const { containerRef, containerClassName } = useBottomAlignedBay(className, `${bayId}:${Children.count(children)}`);
  const { setNodeRef } = useDroppable({ id: bayId, disabled: !isEmpty });
  const { activeId, isValidTarget } = useDragState();
  const { onBayClick } = useBayClick();
  const [isOver, setIsOver] = useState(false);

  useDndMonitor({
    onDragOver(event: DragOverEvent) {
      const { over } = event;
      setIsOver(!!over && (over.id === bayId || over.data.current?.bayId === bayId));
    },
    onDragEnd() { setIsOver(false); },
    onDragCancel() { setIsOver(false); },
  });

  const isDragging = activeId !== null;
  const canDrop = isValidTarget(bayId);

  const depthShadow = "inset 2px 2px 4px rgba(0,0,0,0.55), inset -1px -1px 2px rgba(255,255,255,0.07)";
  let hoverStyle: CSSProperties = { boxShadow: depthShadow };
  if (isDragging && isOver) {
    hoverStyle = canDrop
      ? { boxShadow: "inset 0 0 0 2px var(--color-drop-valid)" }
      : { boxShadow: "inset 0 0 0 2px var(--color-drop-invalid)" };
  }

  return (
    <div
      ref={(node) => {
        setNodeRef(node);
        containerRef(node);
      }}
      className={containerClassName}
      data-strip-scroll-container="true"
      style={hoverStyle}
      onClick={(e) => {
        if (!isDragging && e.target === e.currentTarget) {
          onBayClick(bayId);
        }
      }}
    >
      {children}
    </div>
  );
}

/**
 * Registers a locked/read-only bay as a droppable target so it can show
 * a red border when a strip is dragged over it.
 * Use this instead of a plain <div> for bays that are not SortableBay instances.
 */
export function DropIndicatorBay({
  bayId,
  className,
  children,
}: {
  bayId: string;
  className?: string;
  children?: ReactNode;
}) {
  const { containerRef, containerClassName } = useBottomAlignedBay(className, `${bayId}:${Children.count(children)}`);
  const { setNodeRef } = useDroppable({ id: bayId });
  const { activeId, isValidTarget } = useDragState();
  const [isOver, setIsOver] = useState(false);

  useDndMonitor({
    onDragOver(event: DragOverEvent) {
      setIsOver(!!event.over && event.over.id === bayId);
    },
    onDragEnd() { setIsOver(false); },
    onDragCancel() { setIsOver(false); },
  });

  const isDragging = activeId !== null;
  const canDrop = isValidTarget(bayId);

  const depthShadow = "inset 2px 2px 4px rgba(0,0,0,0.55), inset -1px -1px 2px rgba(255,255,255,0.07)";
  let hoverStyle: CSSProperties = { boxShadow: depthShadow };
  if (isDragging && isOver) {
    hoverStyle = canDrop
      ? { boxShadow: "inset 0 0 0 2px var(--color-drop-valid)" }
      : { boxShadow: "inset 0 0 0 2px var(--color-drop-invalid)" };
  }

  return (
    <div
      ref={(node) => {
        setNodeRef(node);
        containerRef(node);
      }}
      className={containerClassName}
      style={hoverStyle}
    >
      {children}
    </div>
  );
}

export function SortableStrip({
  callsign,
  children,
  hideWhenDragging = false,
  bayId,
  dragDisabled = false,
}: {
  callsign: string;
  children: ReactNode;
  hideWhenDragging?: boolean;
  bayId?: string;
  /** When true, dragging is disabled for this strip (e.g. owned by another controller). */
  dragDisabled?: boolean;
}) {
  const validationDragDisabled = useWebSocketStore((state) => {
    const strip = state.strips.find((candidate) => candidate.callsign === callsign);
    return isValidationActiveForPosition(strip?.validation_status, state.position);
  });
  const effectiveDragDisabled = dragDisabled || validationDragDisabled;
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: callsign,
    data: bayId != null ? { bayId } : undefined,
    disabled: effectiveDragDisabled,
  });
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? (hideWhenDragging ? 0 : 0.5) : 1,
    cursor: effectiveDragDisabled ? "not-allowed" : undefined,
    touchAction: "auto",
  };
  return (
    <div ref={setNodeRef} style={style} {...attributes} {...listeners}>
      {children}
    </div>
  );
}
