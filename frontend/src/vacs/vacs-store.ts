import { create } from "zustand";
import type { VacsState } from "./types";

interface VacsStore {
  state: VacsState;
  setState: (state: VacsState) => void;
}

export const useVacsStore = create<VacsStore>((set) => ({
  state: { status: "unavailable" },
  setState: (state) => set({ state }),
}));
