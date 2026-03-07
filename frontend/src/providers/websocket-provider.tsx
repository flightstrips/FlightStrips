import {type ReactNode, useEffect, useState} from 'react';
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

  const [wsConnected, setWsConnected] = useState(false);

  // Create the WebSocket client only once using lazy state initialization
  const [wsClient] = useState<WebSocketClient>(() => createWebSocketClient(url, {
    onConnected: () => setWsConnected(true),
    onDisconnected: () => setWsConnected(false)
  }));

  // Set the authentication token and connect when it's available
  useEffect(() => {
    const getAndSetTokenAndConnect = async () => {
      if (isAuthenticated && !isLoading && wsClient) {
        try {
          const token = await getAccessTokenSilently();
          wsClient.setToken(token);

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

    // Set up an interval to refresh the token periodically
    const tokenRefreshInterval = setInterval(getAndSetTokenAndConnect, 1000 * 60 * 30); // Refresh every 30 minutes

    return () => {
      clearInterval(tokenRefreshInterval);
      if (wsClient) {
        wsClient.disconnect();
      }
    };
  }, [getAccessTokenSilently, isAuthenticated, isLoading, wsClient]);

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
