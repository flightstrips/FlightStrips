import { create } from "zustand";
import { useEffect } from "react";
import { Bay } from "@/api/models";
import { useStrip } from "@/store/store-hooks";

// Per-browser local mini/hide state: in-memory only, so it resets on every page refresh.
// Clean up keys persisted by earlier versions.
try {
  localStorage.removeItem("flightstrips.localHiddenStrips");
  localStorage.removeItem("flightstrips.localShownStrips");
} catch {
  // Storage unavailable: nothing to clean up.
}

interface LocalHiddenStripsState {
  /** Strips explicitly hidden or minimized locally. */
  hidden: string[];
  /** Strips explicitly unminimized while they would otherwise be minimized by default. */
  shown: string[];
  /** Number of mounted views that render planned departures as half strips by default. */
  defaultHidePlannedViews: number;
  setHidden: (callsign: string, hide: boolean) => void;
  unhide: (callsigns: string[]) => void;
  registerDefaultHidePlanned: () => () => void;
}

export const useLocalHiddenStrips = create<LocalHiddenStripsState>((set, get) => ({
  hidden: [],
  shown: [],
  defaultHidePlannedViews: 0,
  setHidden: (callsign, hide) => {
    const { hidden: currentHidden, shown: currentShown } = get();
    const hidden = hide
      ? (currentHidden.includes(callsign) ? currentHidden : [...currentHidden, callsign])
      : currentHidden.filter((c) => c !== callsign);
    const shown = hide
      ? currentShown.filter((c) => c !== callsign)
      : (currentShown.includes(callsign) ? currentShown : [...currentShown, callsign]);
    set({ hidden, shown });
  },
  unhide: (callsigns) => {
    const callsignsToUnhide = new Set(callsigns);
    set((state) => ({ hidden: state.hidden.filter((callsign) => !callsignsToUnhide.has(callsign)) }));
  },
  registerDefaultHidePlanned: () => {
    set((s) => ({ defaultHidePlannedViews: s.defaultHidePlannedViews + 1 }));
    return () => set((s) => ({ defaultHidePlannedViews: Math.max(0, s.defaultHidePlannedViews - 1) }));
  },
}));

/** Makes planned departures render as half strips by default while the enabled view is mounted. */
export function useDefaultHidePlannedDepartures(enabled = true) {
  const register = useLocalHiddenStrips((s) => s.registerDefaultHidePlanned);
  useEffect(() => enabled ? register() : undefined, [enabled, register]);
}

export function useIsLocallyHidden(callsign: string) {
  const bay = useStrip(callsign)?.bay;
  const explicitlyHidden = useLocalHiddenStrips((s) => s.hidden.includes(callsign));
  const explicitlyShown = useLocalHiddenStrips((s) => s.shown.includes(callsign));
  const defaultHide = useLocalHiddenStrips((s) => s.defaultHidePlannedViews > 0) && bay === Bay.NotCleared;
  if (explicitlyHidden) return true;
  return defaultHide && !explicitlyShown;
}
