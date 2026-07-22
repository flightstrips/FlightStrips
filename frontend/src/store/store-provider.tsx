import { useEffect, useState, type ReactNode } from 'react';
import { useStore } from 'zustand';
import { createWebSocketStore } from './store.ts';
import { WebSocketClient } from '../api/websocket.ts';
import { WebSocketStoreContext } from './store-context.ts';

interface WebSocketStoreProviderProps {
  children: ReactNode;
  wsClient: WebSocketClient;
  connected: boolean;
}

export const WebSocketStoreProvider = ({ children, wsClient, connected }: WebSocketStoreProviderProps) => {
  const [store] = useState(() => createWebSocketStore(wsClient));

  const initialized = useStore(store, state => state.isInitialized);
  const readOnly = useStore(store, state => state.readOnly);

  useEffect(() => {
    store.getState().setAMANConnectionState(connected ? "connected" : "disconnected");
  }, [connected, store]);

  if (!connected) {
    return (
      <div className="w-screen min-h-svh flex flex-col justify-center items-center bg-primary text-white">
        <div className="flex items-center text-4xl font-semibold">
          <span className="inline-flex items-center">Connecting to <svg className="w-8 h-8 mx-2" viewBox="0 0 28 28" fill="none"><rect x="3" y="6" width="22" height="4" rx="1" fill="#a0dae4" /><rect x="3" y="12" width="22" height="4" rx="1" fill="#a0dae4" opacity="0.7" /><rect x="3" y="18" width="22" height="4" rx="1" fill="#a0dae4" opacity="0.4" /></svg>FlightStrips</span>
          <span className="ml-2 animate-bounce text-5xl">...</span>
        </div>
      </div>
    );
  }

  if (!initialized) {
    return (
      <div className="w-screen min-h-svh flex flex-col justify-center items-center bg-primary text-white">
        <a className="flex items-center text-4xl font-semibold text-white"><svg className="w-12 h-12" viewBox="0 0 28 28" fill="none"><rect x="3" y="6" width="22" height="4" rx="1" fill="#a0dae4"></rect><rect x="3" y="12" width="22" height="4" rx="1" fill="#a0dae4" opacity="0.7"></rect><rect x="3" y="18" width="22" height="4" rx="1" fill="#a0dae4" opacity="0.4"></rect></svg>FlightStrips</a>
        <br />
        <div className="flex items-center text-4xl font-semibold">
          <span>{readOnly ? "Connected as observer" : "Waiting for ES connection"}</span>
          <span className="ml-2 animate-bounce text-5xl">...</span>
        </div>
        {readOnly && (
          <div className="mt-4 text-lg text-white/80">
            Waiting for an online controller to observe.
          </div>
        )}
      </div>
    );
  }

  return (
    <WebSocketStoreContext.Provider value={store}>
      {children}
    </WebSocketStoreContext.Provider>
  );
};
