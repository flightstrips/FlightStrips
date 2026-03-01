import { createContext, useContext } from "react";

interface DragState {
  activeId: string | null;
  isValidTarget: (bayId: string) => boolean;
}

export const DragStateContext = createContext<DragState>({ activeId: null, isValidTarget: () => true });

export function useDragState() {
  return useContext(DragStateContext);
}
