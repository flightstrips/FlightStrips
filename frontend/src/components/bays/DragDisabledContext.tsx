import { createContext, useContext, type Dispatch, type SetStateAction } from "react";

interface DragDisabledContextValue {
  setDragDisabled: Dispatch<SetStateAction<boolean>>;
}

export const DragDisabledContext = createContext<DragDisabledContextValue>({
  setDragDisabled: () => {},
});

export const useDragDisabled = () => useContext(DragDisabledContext);
