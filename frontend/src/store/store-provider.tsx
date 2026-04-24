import { useState, type ReactNode } from 'react';
import { useStore } from 'zustand';
import { createWebSocketStore } from './store.ts';
import { WebSocketClient } from '../api/websocket.ts';
import { WebSocketStoreContext } from './store-context.ts';

interface WebSocketStoreProviderProps {
  children: ReactNode;
  wsClient: WebSocketClient;
}

export const WebSocketStoreProvider = ({ children, wsClient }: WebSocketStoreProviderProps) => {
  const [store] = useState(() => createWebSocketStore(wsClient));

  const initialized = useStore(store, state => state.isInitialized);
  const readOnly = useStore(store, state => state.readOnly);

  if (!initialized) {
    return (
      <div className="w-screen min-h-svh flex flex-col justify-center items-center bg-primary text-white">
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
