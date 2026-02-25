import {
  DndContext,
  closestCenter,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  verticalListSortingStrategy,
  useSortable,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";

interface SortableBayProps {
  strips: { callsign: string }[];
  onReorder: (callsign: string, before: string | null) => void;
  children: (callsign: string) => React.ReactNode;
  className?: string;
}

export function SortableBay({ strips, onReorder, children, className }: SortableBayProps) {
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } })
    // distance: 5px threshold — short taps register as clicks (selection), not drags
  );

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event;
    if (!over || active.id === over.id) return;
    const overIndex = strips.findIndex(s => s.callsign === over.id);
    const before = strips[overIndex]?.callsign ?? null;
    onReorder(active.id as string, before);
  }

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      <SortableContext items={strips.map(s => s.callsign)} strategy={verticalListSortingStrategy}>
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

export function SortableStrip({ callsign, children }: { callsign: string; children: React.ReactNode }) {
  const { attributes, listeners, setNodeRef, transform, transition } = useSortable({ id: callsign });
  const style = { transform: CSS.Transform.toString(transform), transition };
  return (
    <div ref={setNodeRef} style={style} {...attributes} {...listeners}>
      {children}
    </div>
  );
}
