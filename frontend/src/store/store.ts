import { createStore } from 'zustand/vanilla';
import { produce } from 'immer';
import {
  type FrontendStrip,
  type FrontendController,
  type RunwayConfiguration,
  EventType,
  type FrontendInitialEvent,
  type FrontendStripUpdateEvent,
  type FrontendControllerOnlineEvent,
  type FrontendControllerOfflineEvent,
  type FrontendAssignedSquawkEvent,
  type FrontendSquawkEvent,
  type FrontendRequestedAltitudeEvent, type FrontendClearedAltitudeEvent, type FrontendBayEvent
} from '../api/models.ts';
import { WebSocketClient } from '../api/websocket.ts';

// Define the state interface for our store
export interface WebSocketState {
  controllers: FrontendController[];
  strips: FrontendStrip[];
  position: string;
  airport: string;
  callsign: string;
  runwaySetup: RunwayConfiguration;
  isInitialized: boolean;
}

// Create the store using createVanilla
export const createWebSocketStore = (wsClient: WebSocketClient) => {
  // Initial state
  const initialState: WebSocketState = {
    controllers: [],
    strips: [],
    position: '',
    airport: '',
    callsign: '',
    runwaySetup: {
      departure: [],
      arrival: []
    },
    isInitialized: false
  };

  // Create the store
  const store = createStore<WebSocketState>()(() => initialState);

  // Private methods to handle WebSocket events
  const handleInitialEvent = (data: FrontendInitialEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        state.controllers = data.controllers;
        state.strips = data.strips;
        state.position = data.position;
        state.airport = data.airport;
        state.callsign = data.callsign;
        state.runwaySetup = data.runway_setup;
        state.isInitialized = true;
      })
    );
  };

  const handleStripUpdateEvent = (data: FrontendStripUpdateEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        const stripIndex = state.strips.findIndex(strip => strip.callsign === data.callsign);

        if (stripIndex !== -1) {
          // Update existing strip
          state.strips[stripIndex] = {
            ...data
          };
        } else {
          // Add new strip
          state.strips.push(data);
        }
      })
    );
  };

  const handleControllerOnlineEvent = (data: FrontendControllerOnlineEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        const controllerIndex = state.controllers.findIndex(
          controller => controller.callsign === data.callsign
        );

        if (controllerIndex === -1) {
          // Add new controller
          state.controllers.push({
            callsign: data.callsign,
            position: data.position
          });
        }
      })
    );
  };

  const handleControllerOfflineEvent = (data: FrontendControllerOfflineEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        state.controllers = state.controllers.filter(
          controller => controller.callsign !== data.callsign
        );
      })
    );
  };

  const handleAssignedSquawkEvent = (data: FrontendAssignedSquawkEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        const stripIndex = state.strips.findIndex(strip => strip.callsign === data.callsign);

        if (stripIndex !== -1) {
          state.strips[stripIndex].assigned_squawk = data.squawk;
        }
      })
    );
  };

  const handleSquawkEvent = (data: FrontendSquawkEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        const stripIndex = state.strips.findIndex(strip => strip.callsign === data.callsign);

        if (stripIndex !== -1) {
          state.strips[stripIndex].squawk = data.squawk;
        }
      })
    );
  };

  const handleRequestedAltitudeEvent = (data: FrontendRequestedAltitudeEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        const stripIndex = state.strips.findIndex(strip => strip.callsign === data.callsign);

        if (stripIndex !== -1) {
          state.strips[stripIndex].requested_altitude = data.altitude;
        }
      })
    );
  };

  const handleClearedAltitudeEvent = (data: FrontendClearedAltitudeEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        const stripIndex = state.strips.findIndex(strip => strip.callsign === data.callsign);

        if (stripIndex !== -1) {
          state.strips[stripIndex].cleared_altitude = data.altitude;
        }
      })
    );
  };

  const handleBayEvent = (data: FrontendBayEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        const stripIndex = state.strips.findIndex(strip => strip.callsign === data.callsign);

        if (stripIndex !== -1) {
          state.strips[stripIndex].bay = data.bay;
        }
      })
    );
  };

  // Register event handlers
  wsClient.on(EventType.FrontendInitial, handleInitialEvent);
  wsClient.on(EventType.FrontendStripUpdate, handleStripUpdateEvent);
  wsClient.on(EventType.FrontendControllerOnline, handleControllerOnlineEvent);
  wsClient.on(EventType.FrontendControllerOffline, handleControllerOfflineEvent);
  wsClient.on(EventType.FrontendAssignedSquawk, handleAssignedSquawkEvent);
  wsClient.on(EventType.FrontendSquawk, handleSquawkEvent);
  wsClient.on(EventType.FrontendRequestedAltitude, handleRequestedAltitudeEvent);
  wsClient.on(EventType.FrontendClearedAltitude, handleClearedAltitudeEvent);
  wsClient.on(EventType.FrontendBay, handleBayEvent);

  return store;
};
