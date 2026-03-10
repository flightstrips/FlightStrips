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

const LOWER_FREQUENCIES = new Set(["119.905", "121.630", "121.905", "121.730"]);

export const useLowerPositionOnline = () =>
  useWebSocketStore((state) =>
    state.controllers.some((c) => LOWER_FREQUENCIES.has(c.position) && c.position !== state.position)
  );

export const useDelOnline = () =>
  useWebSocketStore((state) =>
    state.controllers.some(
      (c) => c.position !== state.position && c.position === "119.905"
    )
  );

export const useApronOnline = () =>
  useWebSocketStore((state) =>
    state.controllers.some(
      (c) =>
        c.position !== state.position &&
        (c.position === "121.630" || c.position === "121.905" || c.position === "121.730")
    )
  );

export const useCtwrOnline = () =>
  useWebSocketStore((state) =>
    state.controllers.some(
      (c) => c.position !== state.position && c.position === "118.580"
    )
  );

export const useLayoutChooserOpen = () => useWebSocketStore((state) => state.layoutChooserOpen);
export const useSetLayoutChooserOpen = () => useWebSocketStore((state) => state.setLayoutChooserOpen);
