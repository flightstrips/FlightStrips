import {type ReactNode, useCallback, useEffect, useRef, useState} from 'react';
import {WebSocketClient, createWebSocketClient} from '@/api/websocket';
import {WebSocketStoreProvider} from '@/store/store-provider';
import {UserRatingContext} from '@/store/user-rating-context';
import {useAuth0} from '@auth0/auth0-react';

interface WebSocketProviderProps {
  children: ReactNode;
  url: string;
}

// Match the Auth0 SDK cache policy and refresh once the token enters its last minute.
const REFRESH_BUFFER_MS = 60 * 1000;
const MIN_REFRESH_RETRY_MS = 1000;

function decodeJwtPayload(token: string): Record<string, unknown> | null {
  try {
    const base64Url = token.split('.')[1];
    const base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/');
    return JSON.parse(atob(base64)) as Record<string, unknown>;
  } catch {
    return null;
  }
}

// Decode the JWT payload and return the expiry as a Unix timestamp (ms), or null on failure.
function getTokenExpiryMs(token: string): number | null {
  const payload = decodeJwtPayload(token);
  if (payload && typeof payload.exp === 'number') {
    return payload.exp * 1000;
  }
  return null;
}

function getTokenRating(token: string): number {
  const payload = decodeJwtPayload(token);
  if (payload && typeof payload['vatsim/rating'] === 'number') {
    return payload['vatsim/rating'] as number;
  }
  return 0;
}

function getTokenRefreshDelayMs(token: string, nowMs = Date.now()): number | null {
  const expiryMs = getTokenExpiryMs(token);
  if (expiryMs === null) {
    return null;
  }

  const remainingMs = expiryMs - nowMs;
  if (remainingMs <= 0) {
    return MIN_REFRESH_RETRY_MS;
  }

  if (remainingMs > REFRESH_BUFFER_MS) {
    return remainingMs - REFRESH_BUFFER_MS;
  }

  return Math.max(Math.floor(remainingMs / 2), MIN_REFRESH_RETRY_MS);
}

export const WebSocketProvider = ({children, url}: WebSocketProviderProps) => {
  // Get the authentication token from Auth0
  const {getAccessTokenSilently, isAuthenticated, isLoading} = useAuth0();

  const [wsConnected, setWsConnected] = useState(false);
  const [userRating, setUserRating] = useState(0);

  // Create the WebSocket client only once using lazy state initialization
  const [wsClient] = useState<WebSocketClient>(() => createWebSocketClient(url, {
    onConnected: () => setWsConnected(true),
    onDisconnected: () => setWsConnected(false)
  }));

  const refreshTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Stable ref so scheduleTokenRefresh can always call the latest refreshToken without
  // creating a circular dependency between the two useCallback hooks.
  const refreshTokenRef = useRef<() => Promise<void>>(async () => {});

  const scheduleRefreshRetry = useCallback(() => {
    if (refreshTimeoutRef.current) {
      clearTimeout(refreshTimeoutRef.current);
    }

    console.warn(`Retrying token refresh in ${Math.round(MIN_REFRESH_RETRY_MS / 1000)}s`);
    refreshTimeoutRef.current = setTimeout(() => refreshTokenRef.current(), MIN_REFRESH_RETRY_MS);
  }, []);

  const scheduleTokenRefresh = useCallback((token: string) => {
    if (refreshTimeoutRef.current) {
      clearTimeout(refreshTimeoutRef.current);
      refreshTimeoutRef.current = null;
    }

    const refreshInMs = getTokenRefreshDelayMs(token);
    if (refreshInMs === null) return;

    console.log(`Scheduling token refresh in ${Math.round(refreshInMs / 1000)}s`);
    refreshTimeoutRef.current = setTimeout(() => refreshTokenRef.current(), refreshInMs);
  }, []);

  const refreshToken = useCallback(async () => {
    if (!isAuthenticated) return;
    try {
      const token = await getAccessTokenSilently();
      wsClient.setToken(token);
      setUserRating(getTokenRating(token));
      scheduleTokenRefresh(token);
    } catch (error) {
      console.error('Error refreshing access token:', error);
      scheduleRefreshRetry();
    }
  }, [getAccessTokenSilently, isAuthenticated, wsClient, scheduleRefreshRetry, scheduleTokenRefresh]);

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
          setUserRating(getTokenRating(token));
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
  }, [getAccessTokenSilently, isAuthenticated, isLoading, wsClient, scheduleTokenRefresh]);

  useEffect(() => {
    return () => {
      if (refreshTimeoutRef.current) {
        clearTimeout(refreshTimeoutRef.current);
      }
      wsClient.disconnect();
    };
  }, [wsClient]);

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
    <UserRatingContext.Provider value={userRating}>
      <WebSocketStoreProvider wsClient={wsClient}>
        {children}
      </WebSocketStoreProvider>
    </UserRatingContext.Provider>
  );
};
