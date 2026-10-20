import { useState, type ReactNode } from 'react';
import { fn } from 'storybook/test';
import { WebSocketClient } from '@/api/websocket';
import { createWebSocketStore } from '@/store/store';
import { WebSocketStoreContext } from '@/store/store-context';

class StoryClient extends WebSocketClient {
  override send = fn().mockName('Send operational action');
  override connect = async () => {};
  override reconnect = () => {};
}

export function StoryStore({ children }: { children: ReactNode }) {
  const [store] = useState(() => {
    const store = createWebSocketStore(new StoryClient('ws://storybook.invalid'));
    store.setState({
      position: 'EKCH_TWR', airport: 'EKCH', callsign: 'EKCH_TWR',
      controllers: [
        { callsign: 'EKCH_TWR', position: 'EKCH_TWR', identifier: 'TE', section: 'TWR', owned_sectors: ['TE'] },
        { callsign: 'EKCH_GND', position: 'EKCH_GND', identifier: 'GE', section: 'GND', owned_sectors: ['GE'] },
      ],
      runwaySetup: { departure: ['22R'], arrival: ['22L'] },
    });
    return store;
  });
  return <WebSocketStoreContext.Provider value={store}>{children}</WebSocketStoreContext.Provider>;
}
