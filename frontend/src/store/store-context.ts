import { createContext } from 'react';
import { createWebSocketStore } from './store.ts';

export const WebSocketStoreContext = createContext<ReturnType<typeof createWebSocketStore> | null>(null);
