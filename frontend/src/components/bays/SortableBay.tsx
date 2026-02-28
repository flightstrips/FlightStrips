import {
  DndContext,
  closestCenter,
  PointerSensor,
  useSensor,
  useSensors,
  useDroppable,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  verticalListSortingStrategy,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useDragState } from "@/components/bays/ViewDndContext.tsx";
import type { CSSProperties, ReactNode } from "react";

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
          <SortableStrip key={s.callsign} callsign={s.callsign} hideWhenDragging>
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

  const isDragging = activeId !== null;
  const canDrop = isValidTarget(bayId);
  const invalid = isDragging && !canDrop;

  const invalidStyle: CSSProperties = invalid
    ? { boxShadow: "inset 0 0 0 2px #ef4444", backgroundColor: "rgba(239,68,68,0.15)" }
    : {};

  return (
    <div ref={setNodeRef} className={className} style={invalidStyle}>
      {children}
    </div>
  );
}

export function SortableStrip({ callsign, children, hideWhenDragging = false }: { callsign: string; children: ReactNode; hideWhenDragging?: boolean }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: callsign });
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