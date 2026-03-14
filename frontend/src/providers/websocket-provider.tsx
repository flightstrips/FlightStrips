import {type ReactNode, useCallback, useEffect, useRef, useState} from 'react';
import {WebSocketClient, createWebSocketClient} from '@/api/websocket';
import {WebSocketStoreProvider} from '@/store/store-provider';
import {useAuth0} from '@auth0/auth0-react';

interface WebSocketProviderProps {
  children: ReactNode;
  url: string;
}

// How early to refresh before the token expires, matching the EuroScope plugin behaviour.
const REFRESH_BUFFER_MS = 30 * 60 * 1000;

// Decode the JWT payload and return the expiry as a Unix timestamp (ms), or null on failure.
function getTokenExpiryMs(token: string): number | null {
  try {
    const base64Url = token.split('.')[1];
    const base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/');
    const payload = JSON.parse(atob(base64)) as Record<string, unknown>;
    if (typeof payload.exp === 'number') {
      return payload.exp * 1000;
    }
    return null;
  } catch {
    return null;
  }
}

export const WebSocketProvider = ({children, url}: WebSocketProviderProps) => {
  // Get the authentication token from Auth0
  const {getAccessTokenSilently, isAuthenticated, isLoading} = useAuth0();

  const [wsConnected, setWsConnected] = useState(false);

  // Create the WebSocket client only once using lazy state initialization
  const [wsClient] = useState<WebSocketClient>(() => createWebSocketClient(url, {
    onConnected: () => setWsConnected(true),
    onDisconnected: () => setWsConnected(false)
  }));

  const refreshTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Stable ref so scheduleTokenRefresh can always call the latest refreshToken without
  // creating a circular dependency between the two useCallback hooks.
  const refreshTokenRef = useRef<() => Promise<void>>(async () => {});

  const scheduleTokenRefresh = useCallback((token: string) => {
    if (refreshTimeoutRef.current) {
      clearTimeout(refreshTimeoutRef.current);
      refreshTimeoutRef.current = null;
    }

    const expiryMs = getTokenExpiryMs(token);
    if (expiryMs === null) return;

    const refreshInMs = expiryMs - Date.now() - REFRESH_BUFFER_MS;
    if (refreshInMs <= 0) {
      // Token is already about to expire — refresh immediately on next tick.
      console.warn('Token is expiring soon; refreshing immediately.');
      refreshTimeoutRef.current = setTimeout(() => refreshTokenRef.current(), 0);
    } else {
      console.log(`Scheduling token refresh in ${Math.round(refreshInMs / 1000)}s`);
      refreshTimeoutRef.current = setTimeout(() => refreshTokenRef.current(), refreshInMs);
    }
  }, []);

  const refreshToken = useCallback(async () => {
    if (!isAuthenticated) return;
    try {
      const token = await getAccessTokenSilently();
      wsClient.setToken(token);
      scheduleTokenRefresh(token);
    } catch (error) {
      console.error('Error refreshing access token:', error);
    }
  }, [getAccessTokenSilently, isAuthenticated, wsClient, scheduleTokenRefresh]);

  // Keep the ref pointing at the latest refreshToken closure.
  useEffect(() => {
    refreshTokenRef.current = refreshToken;
  }, [refreshToken]);

  // Set the authentication token and connect when it's available
  useEffect(() => {
    const getAndSetTokenAndConnect = async () => {
      if (isAuthenticated && !isLoading && wsClient) {
        try {
          const token = await getAccessTokenSilently();
          wsClient.setToken(token);
          scheduleTokenRefresh(token);

          if (!wsClient.isConnected()) {
            wsClient.connect().catch(error => {
              console.error('Failed to connect to WebSocket server:', error);
            });
          }
        } catch (error) {
          console.error('Error getting access token:', error);
        }
      }
    };

    getAndSetTokenAndConnect();

    return () => {
      if (refreshTimeoutRef.current) {
        clearTimeout(refreshTimeoutRef.current);
      }
      if (wsClient) {
        wsClient.disconnect();
      }
    };
  }, [getAccessTokenSilently, isAuthenticated, isLoading, wsClient, scheduleTokenRefresh]);

  if (isLoading) {
    return (<div
      className='w-screen min-h-svh flex justify-center items-center bg-primary text-white text-4xl font-semibold'>Loading...</div>);
  }

  if (!wsConnected) {
    return (
      <div className="w-screen min-h-svh flex flex-col justify-center items-center bg-primary text-white">
        <div className="flex items-center text-4xl font-semibold">
          <span>Connecting to FlightStrips</span>
          <span className="ml-2 animate-bounce text-5xl">...</span>
        </div>
      </div>
    );
  }

  // Provide both the WebSocket client and the store
  return (
    <WebSocketStoreProvider wsClient={wsClient}>
      {children}
    </WebSocketStoreProvider>
  );
};
