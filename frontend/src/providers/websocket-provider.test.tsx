import {act, cleanup, render} from '@testing-library/react';
import type {ReactNode} from 'react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {WebSocketProvider} from './websocket-provider';

const {getToken, logout, client} = vi.hoisted(() => ({
  getToken: vi.fn(),
  logout: vi.fn(),
  client: {
    setToken: vi.fn(),
    connect: vi.fn(),
    reconnect: vi.fn(),
    disconnect: vi.fn(),
    isConnected: vi.fn(),
  },
}));

vi.mock('@auth0/auth0-react', () => ({
  useAuth0: () => ({getAccessTokenSilently: getToken, logout, isAuthenticated: true, isLoading: false}),
}));
vi.mock('@/api/websocket', () => ({createWebSocketClient: () => client}));
vi.mock('@/store/store-provider', () => ({
  WebSocketStoreProvider: ({children}: {children: ReactNode}) => children,
}));

describe('WebSocket authentication', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers();
    logout.mockResolvedValue(undefined);
    client.connect.mockResolvedValue(undefined);
    client.isConnected.mockReturnValue(false);
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.spyOn(console, 'log').mockImplementation(() => {});
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('clears an expired session without opening an unauthenticated socket', async () => {
    getToken.mockResolvedValue(undefined);

    await act(async () => {
      render(<WebSocketProvider url="ws://localhost/events"><span>Board</span></WebSocketProvider>);
    });

    expect(client.setToken).not.toHaveBeenCalled();
    expect(client.connect).not.toHaveBeenCalled();
    expect(client.disconnect).toHaveBeenCalledTimes(1);
    expect(logout).toHaveBeenCalledWith({openUrl: false});
  });

  it('disconnects and requests login when a scheduled refresh returns no token', async () => {
    const token = `header.${btoa(JSON.stringify({exp: Math.floor(Date.now() / 1000) + 120}))}.signature`;
    getToken.mockResolvedValueOnce(token).mockResolvedValue(undefined);

    await act(async () => {
      render(<WebSocketProvider url="ws://localhost/events"><span>Board</span></WebSocketProvider>);
    });
    expect(client.setToken).toHaveBeenCalledExactlyOnceWith(token);
    expect(client.connect).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000);
    });

    expect(client.setToken).toHaveBeenCalledTimes(1);
    expect(client.disconnect).toHaveBeenCalledTimes(1);
    expect(logout).toHaveBeenCalledWith({openUrl: false});
    expect(vi.getTimerCount()).toBe(0);
  });
  it('ignores an old board token lookup that resolves after unmount', async () => {
    let resolve!: (token: string) => void;
    getToken.mockImplementation(() => new Promise<string>(done => {resolve = done;}));
    const view = render(<WebSocketProvider url="ws://old"><span>Old board</span></WebSocketProvider>);
    view.unmount();
    await act(async () => {resolve("old-token");});
    expect(client.setToken).not.toHaveBeenCalled();
    expect(client.connect).not.toHaveBeenCalled();
    expect(logout).not.toHaveBeenCalled();
    expect(vi.getTimerCount()).toBe(0);
  });

  it('requests an authorized replacement snapshot when the same own membership becomes ready again', async () => {
    getToken.mockResolvedValue("token");
    const view = render(<WebSocketProvider url="ws://board" ownPluginAvailable={false}><span>Board</span></WebSocketProvider>);
    await act(async () => {});
    client.isConnected.mockReturnValue(true);
    view.rerender(<WebSocketProvider url="ws://board" ownPluginAvailable={true}><span>Board</span></WebSocketProvider>);
    expect(client.reconnect).toHaveBeenCalledOnce();
    view.rerender(<WebSocketProvider url="ws://board" ownPluginAvailable={true}><span>Board</span></WebSocketProvider>);
    expect(client.reconnect).toHaveBeenCalledOnce();
  });

});
