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
export const usePosition = () => useWebSocketStore((state) => state.position);
export const useAirport = () => useWebSocketStore((state) => state.airport);
export const useCallsign = () => useWebSocketStore((state) => state.callsign);
export const useRunwaySetup = () => useWebSocketStore((state) => state.runwaySetup);
export const useStrip = (callsign: string) => useWebSocketStore((state) => state.strips.find(strip => strip.callsign === callsign));
export const useSelectedCallsign = () => useWebSocketStore((state) => state.selectedCallsign);
export const useSelectStrip = () => useWebSocketStore((state) => state.selectStrip);
