import {closestCenter, pointerWithin, rectIntersection, type CollisionDetection} from "@dnd-kit/core";
import {RADIS_BAYS} from "./bays";

const isBay = (id: string | number) => RADIS_BAYS.some(bay => bay === id);

// Resolve the visible bay before its strips. A scrolled strip retains a measured
// rectangle beyond its clipping panel and must never capture another bay's drop.
export const radisCollisionDetection: CollisionDetection = args => {
  const panels = args.droppableContainers.filter(c => isBay(c.id) || c.id === "RADIS_REMOVE");
  const panelArgs = {...args, droppableContainers: panels};
  const panelHits = args.pointerCoordinates ? pointerWithin(panelArgs) : rectIntersection(panelArgs);
  const panel = panelHits[0];
  if (!panel || !isBay(panel.id)) return panelHits;

  const bounds = args.droppableRects.get(panel.id);
  const strips = args.droppableContainers.filter(c => {
    if (isBay(c.id) || c.data.current?.radisBay !== panel.id) return false;
    const rect = args.droppableRects.get(c.id);
    return rect && bounds && rect.bottom > bounds.top && rect.top < bounds.bottom;
  });
  const stripArgs = {...args, droppableContainers: strips};
  const stripHits = args.pointerCoordinates ? pointerWithin(stripArgs) : closestCenter(stripArgs);
  return stripHits.length ? stripHits : panelHits;
};
