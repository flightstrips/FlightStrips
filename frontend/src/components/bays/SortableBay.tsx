import {
  DndContext,
  closestCenter,
  PointerSensor,
  useSensor,
  useSensors,
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
import { useState, type CSSProperties, type ReactNode } from "react";

interface SortableBayProps {
  strips: { callsign: string }[];
  /** Required when standalone=true (default). Not used when standalone=false. */
  onReorder?: (callsign: string, before: string | null) => void;
  children: (callsign: string) => ReactNode;
  className?: string;
  /** Unique bay identifier. Required when standalone=false so the container is droppable. */
  bayId?: string;
  /**
   * When true (default) wraps strips in its own DndContext for self-contained reordering.
   * When false, relies on a parent ViewDndContext; bayId must be provided.
   */
  standalone?: boolean;
}

export function SortableBay({
  strips,
  onReorder,
  children,
  className,
  bayId,
  standalone = true,
}: SortableBayProps) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } })
    // distance: 5px threshold — short taps register as clicks, not drags
  );

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event;
    if (!over || active.id === over.id || !onReorder) return;
    const overIndex = strips.findIndex(s => s.callsign === over.id);
    const before = strips[overIndex]?.callsign ?? null;
    onReorder(active.id as string, before);
  }

  const sortableItems = strips.map(s => s.callsign);

  if (standalone) {
    return (
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
        <SortableContext items={sortableItems} strategy={verticalListSortingStrategy}>
          <div className={className}>
            {strips.map(s => (
              <SortableStrip key={s.callsign} callsign={s.callsign}>
                {children(s.callsign)}
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
      <DroppableContainer bayId={bayId!} className={className}>
        {strips.map(s => (
          <SortableStrip key={s.callsign} callsign={s.callsign} hideWhenDragging bayId={bayId}>
            {children(s.callsign)}
          </SortableStrip>
        ))}
      </DroppableContainer>
    </SortableContext>
  );
}

function DroppableContainer({
  bayId,
  className,
  children,
}: {
  bayId: string;
  className?: string;
  children: ReactNode;
}) {
  const { setNodeRef } = useDroppable({ id: bayId });
  const { activeId, isValidTarget } = useDragState();
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

  let hoverStyle: CSSProperties = {};
  if (isDragging && isOver) {
    hoverStyle = canDrop
      ? { boxShadow: "inset 0 0 0 2px #FFFB03" }
      : { boxShadow: "inset 0 0 0 2px #ef4444" };
  }

  return (
    <div ref={setNodeRef} className={className} style={hoverStyle}>
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

  let hoverStyle: CSSProperties = {};
  if (isDragging && isOver) {
    hoverStyle = canDrop
      ? { boxShadow: "inset 0 0 0 2px #FFFB03" }
      : { boxShadow: "inset 0 0 0 2px #ef4444" };
  }

  return (
    <div ref={setNodeRef} className={className} style={hoverStyle}>
      {children}
    </div>
  );
}

export function SortableStrip({
  callsign,
  children,
  hideWhenDragging = false,
  bayId,
}: {
  callsign: string;
  children: ReactNode;
  hideWhenDragging?: boolean;
  bayId?: string;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: callsign,
    data: bayId != null ? { bayId } : undefined,
  });
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? (hideWhenDragging ? 0 : 0.5) : 1,
  };
  return (
    <div ref={setNodeRef} style={style} {...attributes} {...listeners}>
      {children}
    </div>
  );
}
