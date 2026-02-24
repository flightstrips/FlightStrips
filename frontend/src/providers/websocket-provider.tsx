import {type ReactNode, useEffect, useRef, useState} from 'react';
import {WebSocketClient, createWebSocketClient} from '@/api/websocket';
import {WebSocketStoreProvider} from '@/store/store-provider';
import {useAuth0} from '@auth0/auth0-react';

interface WebSocketProviderProps {
  children: ReactNode;
  url: string;
}

export const WebSocketProvider = ({children, url}: WebSocketProviderProps) => {
  // Get the authentication token from Auth0
  const {getAccessTokenSilently, isAuthenticated, isLoading} = useAuth0();

  // Create the WebSocket client only once
  const wsClientRef = useRef<WebSocketClient | null>(null);

  const [wsConnected, setWsConnected] = useState(false);

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
      if (wsClientRef.current) {
        wsClientRef.current.disconnect();
      }
    };
  }, [getAccessTokenSilently, isAuthenticated, isLoading]);

  if (isLoading) {
    return (<div
      className='w-screen min-h-svh flex justify-center items-center bg-primary text-white text-4xl font-semibold'>Loading...</div>);
  }

  if (!wsClientRef.current) {
    wsClientRef.current = createWebSocketClient(url, {
      onConnected: () => setWsConnected(true),
      onDisconnected: () => setWsConnected(false)
    });
  }

  // If the WebSocket client hasn't been created yet, don't render anything
  if (!wsClientRef.current) {
    return null;
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
    <WebSocketStoreProvider wsClient={wsClientRef.current}>
      {children}
    </WebSocketStoreProvider>
  );
};

