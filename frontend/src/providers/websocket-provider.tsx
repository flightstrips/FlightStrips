import {type ReactNode, useCallback, useEffect, useRef, useState} from 'react';
import {WebSocketClient, createWebSocketClient} from '@/api/websocket';
import {WebSocketStoreProvider} from '@/store/store-provider';
import {UserRatingContext} from '@/store/user-rating-context';
import {useAuth0} from '@auth0/auth0-react';

interface WebSocketProviderProps {
  children: ReactNode;
  url: string;
}

type AuthErrorLike = {
  error?: string;
  error_description?: string;
  message?: string;
};

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

function requiresLoginPrompt(error: unknown): boolean {
  const authError = typeof error === 'object' && error !== null ? error as AuthErrorLike : null;
  const errorCode = typeof authError?.error === 'string' ? authError.error.toLowerCase() : '';
  const errorDescription = typeof authError?.error_description === 'string' ? authError.error_description.toLowerCase() : '';
  const errorMessage = error instanceof Error
    ? error.message.toLowerCase()
    : typeof authError?.message === 'string'
      ? authError.message.toLowerCase()
      : typeof error === 'string'
        ? error.toLowerCase()
        : '';

  const combinedMessage = `${errorCode} ${errorDescription} ${errorMessage}`;

  return errorCode === 'invalid_grant'
    || errorCode === 'login_required'
    || errorCode === 'missing_refresh_token'
    || combinedMessage.includes('invalid refresh token')
    || combinedMessage.includes('unknown or invalid refresh token')
    || combinedMessage.includes('missing refresh token');
}

export const WebSocketProvider = ({children, url}: WebSocketProviderProps) => {
  // Get the authentication token from Auth0
  const {getAccessTokenSilently, isAuthenticated, isLoading, logout} = useAuth0();

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
  const reauthenticatingRef = useRef(false);

  const clearRefreshTimeout = useCallback(() => {
    if (refreshTimeoutRef.current) {
      clearTimeout(refreshTimeoutRef.current);
      refreshTimeoutRef.current = null;
    }
  }, []);

  const scheduleRefreshRetry = useCallback(() => {
    if (reauthenticatingRef.current) {
      return;
    }

    clearRefreshTimeout();
    console.warn(`Retrying token refresh in ${Math.round(MIN_REFRESH_RETRY_MS / 1000)}s`);
    refreshTimeoutRef.current = setTimeout(() => refreshTokenRef.current(), MIN_REFRESH_RETRY_MS);
  }, [clearRefreshTimeout]);

  const resetAuthenticationState = useCallback(async () => {
    if (reauthenticatingRef.current) {
      return;
    }

    reauthenticatingRef.current = true;
    clearRefreshTimeout();
    wsClient.disconnect();
    setWsConnected(false);

    try {
      // Clear the stale Auth0 cache entry so the existing route guards can prompt for login again.
      await logout({openUrl: false});
    } catch (logoutError) {
      reauthenticatingRef.current = false;
      console.error('Error clearing invalid auth session:', logoutError);
    }
  }, [clearRefreshTimeout, logout, wsClient]);

  const handleAccessTokenError = useCallback(async (error: unknown, retryable: boolean, logMessage: string) => {
    console.error(logMessage, error);

    if (requiresLoginPrompt(error)) {
      await resetAuthenticationState();
      return;
    }

    if (retryable) {
      scheduleRefreshRetry();
    }
  }, [resetAuthenticationState, scheduleRefreshRetry]);

  const scheduleTokenRefresh = useCallback((token: string) => {
    clearRefreshTimeout();

    const refreshInMs = getTokenRefreshDelayMs(token);
    if (refreshInMs === null) return;

    console.log(`Scheduling token refresh in ${Math.round(refreshInMs / 1000)}s`);
    refreshTimeoutRef.current = setTimeout(() => refreshTokenRef.current(), refreshInMs);
  }, [clearRefreshTimeout]);

  const refreshToken = useCallback(async () => {
    if (!isAuthenticated || reauthenticatingRef.current) return;

    try {
      const token = await getAccessTokenSilently();
      wsClient.setToken(token);
      setUserRating(getTokenRating(token));
      scheduleTokenRefresh(token);
    } catch (error) {
      await handleAccessTokenError(error, true, 'Error refreshing access token:');
    }
  }, [getAccessTokenSilently, handleAccessTokenError, isAuthenticated, scheduleTokenRefresh, wsClient]);

  // Keep the ref pointing at the latest refreshToken closure.
  useEffect(() => {
    refreshTokenRef.current = refreshToken;
  }, [refreshToken]);

  // Set the authentication token and connect when it's available
  useEffect(() => {
    const getAndSetTokenAndConnect = async () => {
      if (isAuthenticated && !isLoading && wsClient && !reauthenticatingRef.current) {
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
          await handleAccessTokenError(error, false, 'Error getting access token:');
        }
      }
    };

    void getAndSetTokenAndConnect();
  }, [getAccessTokenSilently, handleAccessTokenError, isAuthenticated, isLoading, scheduleTokenRefresh, wsClient]);

  useEffect(() => {
    if (!isAuthenticated) {
      reauthenticatingRef.current = false;
      clearRefreshTimeout();
    }
  }, [clearRefreshTimeout, isAuthenticated]);

  useEffect(() => {
    return () => {
      clearRefreshTimeout();
      wsClient.disconnect();
    };
  }, [clearRefreshTimeout, wsClient]);

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
