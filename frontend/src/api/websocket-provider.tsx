import { type ReactNode, useEffect, useRef } from 'react';
import { WebSocketClient, createWebSocketClient } from './websocket';
import { WebSocketStoreProvider } from '../store/store-provider.tsx';
import { useAuth0 } from '@auth0/auth0-react';

interface WebSocketProviderProps {
  children: ReactNode;
  url: string;
}

export const WebSocketProvider = ({ children, url }: WebSocketProviderProps) => {
  // Get the authentication token from Auth0
  const { getAccessTokenSilently, isAuthenticated, isLoading } = useAuth0();

  // Create the WebSocket client only once
  const wsClientRef = useRef<WebSocketClient | null>(null);

  // Set the authentication token and connect when it's available
  useEffect(() => {
    const getAndSetTokenAndConnect = async () => {
      if (isAuthenticated && !isLoading && wsClientRef.current) {
        try {
          const token = await getAccessTokenSilently();
          wsClientRef.current.setToken(token);

          if (!wsClientRef.current.isConnected()) {
            wsClientRef.current.connect().catch(error => {
              console.error('Failed to connect to WebSocket server:', error);
            });
          }
        } catch (error) {
          console.error('Error getting access token:', error);
        }
      }
    };

    getAndSetTokenAndConnect();

    // Set up an interval to refresh the token periodically
    const tokenRefreshInterval = setInterval(getAndSetTokenAndConnect, 1000 * 60 * 30); // Refresh every 30 minutes

    return () => {
      clearInterval(tokenRefreshInterval);
      if (wsClientRef.current?.isConnected()) {
        wsClientRef.current.disconnect();
      }
    };
  }, [getAccessTokenSilently, isAuthenticated, isLoading]);

  if (isLoading) {
    // TODO Simon please fix
    return <div>Loading...</div>;
  }

  if (!wsClientRef.current) {
    wsClientRef.current = createWebSocketClient(url);
  }


  // If the WebSocket client hasn't been created yet, don't render anything
  if (!wsClientRef.current) {
    return null;
  }

  // Provide both the WebSocket client and the store
  return (
    <WebSocketStoreProvider wsClient={wsClientRef.current}>
      {children}
    </WebSocketStoreProvider>
  );
};
