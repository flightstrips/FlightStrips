import { useContext } from 'react';
import { useStore } from 'zustand';
import { type WebSocketState } from './store.ts';
import { WebSocketStoreContext } from './store-context.ts';

export const useWebSocketStore = <T,>(selector: (state: WebSocketState) => T): T => {
  const store = useContext(WebSocketStoreContext);

  if (!store) {
    throw new Error('useWebSocketStore must be used within a WebSocketStoreProvider');
  }

  return useStore(store, selector);
};

export const useControllers = () => useWebSocketStore((state) => state.controllers);
export const useStrips = () => useWebSocketStore((state) => state.strips);
export const useTacticalStrips = () => useWebSocketStore((state) => state.tacticalStrips);
export const usePosition = () => useWebSocketStore((state) => state.position);
export const useAirport = () => useWebSocketStore((state) => state.airport);
export const useCallsign = () => useWebSocketStore((state) => state.callsign);
export const useRunwaySetup = () => useWebSocketStore((state) => state.runwaySetup);
export const useStrip = (callsign: string) => useWebSocketStore((state) => state.strips.find(strip => strip.callsign === callsign));
export const useSelectedCallsign = () => useWebSocketStore((state) => state.selectedCallsign);
export const useSelectStrip = () => useWebSocketStore((state) => state.selectStrip);
export const useMessages = () => useWebSocketStore((state) => state.messages);
/** @deprecated use useMessages */
export const useActiveMessages = () => useWebSocketStore((state) => state.messages);
export const useMyPosition = () => useWebSocketStore((state) => state.position);
export const useStripTransfers = () => useWebSocketStore((state) => state.stripTransfers);
export const useMetar = () => useWebSocketStore((state) => state.metar);

const LOWER_SECTIONS = new Set(["DEL", "GND"]);

export const useLowerPositionOnline = () =>
  useWebSocketStore((state) =>
    state.controllers.some((c) => LOWER_SECTIONS.has(c.section) && c.callsign !== state.callsign)
  );

/**
 * Returns true if EKCH_DEL is currently online (other than the current user).
 * Falls back to checking callsign when section is empty (controller_online events
 * do not include section; see store.ts handleControllerOnlineEvent).
 */
export const useDelOnline = () =>
  useWebSocketStore((state) =>
    state.controllers.some(
      (c) =>
        c.callsign !== state.callsign &&
        (c.section === "DEL" || c.callsign === "EKCH_DEL" || c.position === "EKCH_DEL")
    )
  );

/**
 * Returns true if any APRON (GND) position is currently online (other than the current user).
 * Falls back to callsign suffix when section is empty.
 */
export const useApronOnline = () =>
  useWebSocketStore((state) =>
    state.controllers.some(
      (c) =>
        c.callsign !== state.callsign &&
        (c.section === "GND" ||
          c.callsign === "EKCH_A_GND" ||
          c.callsign === "EKCH_B_GND" ||
          c.callsign === "EKCH_C_GND" ||
          c.position === "EKCH_A_GND" ||
          c.position === "EKCH_B_GND" ||
          c.position === "EKCH_C_GND")
    )
  );

/**
 * Returns true if EKCH_C_TWR (CTWR — the position that uses the GEGW layout) is
 * currently online (other than the current user).
 * Note: all TWR positions share section "TWR", so we must check the specific callsign.
 */
export const useCtwrOnline = () =>
  useWebSocketStore((state) =>
    state.controllers.some(
      (c) =>
        c.callsign !== state.callsign &&
        (c.callsign === "EKCH_C_TWR" || c.position === "EKCH_C_TWR")
    )
  );

export const useLayoutChooserOpen = () => useWebSocketStore((state) => state.layoutChooserOpen);
export const useSetLayoutChooserOpen = () => useWebSocketStore((state) => state.setLayoutChooserOpen);
