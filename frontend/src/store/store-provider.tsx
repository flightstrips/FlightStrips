import { createContext, useContext, useRef, type ReactNode } from 'react';
import { useStore } from 'zustand';
import {createWebSocketStore, type WebSocketState} from './store.ts';
import { WebSocketClient } from '../api/websocket.ts';

// Create a context for the WebSocket store
const WebSocketStoreContext = createContext<ReturnType<typeof createWebSocketStore> | null>(null);

interface WebSocketStoreProviderProps {
  children: ReactNode;
  wsClient: WebSocketClient;
}

export const WebSocketStoreProvider = ({ children, wsClient }: WebSocketStoreProviderProps) => {
  // Create the store only once using useRef
  const storeRef = useRef<ReturnType<typeof createWebSocketStore> | null>(null);
  
  if (!storeRef.current) {
    storeRef.current = createWebSocketStore(wsClient);
  }

  const initialized = useStore(storeRef.current!, state => state.isInitialized);

  if (!initialized) {
    return (
      // TODO Simon please fix
      <div>Waiting for ES connection...</div>
    )
  }

  return (
    <WebSocketStoreContext.Provider value={storeRef.current}>
      {children}
    </WebSocketStoreContext.Provider>
  );
};

// Custom hook to use the WebSocket store
export const useWebSocketStore = <T,>(selector: (state: WebSocketState) => T): T => {
  const store = useContext(WebSocketStoreContext);
  
  if (!store) {
    throw new Error('useWebSocketStore must be used within a WebSocketStoreProvider');
  }
  
  return useStore(store, selector);
};

// Convenience hooks for common selectors
export const useControllers = () => useWebSocketStore((state) => state.controllers);
export const useStrips = () => useWebSocketStore((state) => state.strips);
export const usePosition = () => useWebSocketStore((state) => state.position);
export const useAirport = () => useWebSocketStore((state) => state.airport);
export const useCallsign = () => useWebSocketStore((state) => state.callsign);
export const useRunwaySetup = () => useWebSocketStore((state) => state.runwaySetup);