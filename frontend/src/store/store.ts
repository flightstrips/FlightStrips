import {createStore} from 'zustand/vanilla';
import {produce} from 'immer';
import {
  ActionType,
  Bay,
  EventType,
  type FrontendAircraftDisconnectEvent,
  type FrontendAssignedSquawkEvent,
  type FrontendBayEvent,
  type FrontendClearedAltitudeEvent,
  type FrontendCommunicationTypeEvent,
  type FrontendController,
  type FrontendControllerOfflineEvent,
  type FrontendControllerOnlineEvent,
  type FrontendInitialEvent,
  type FrontendRequestedAltitudeEvent,
  type FrontendSetHeadingEvent,
  type FrontendSquawkEvent,
  type FrontendStandEvent,
  type FrontendStrip,
  type FrontendStripUpdateEvent,
  type RunwayConfiguration
} from '../api/models.ts';
import {WebSocketClient} from '../api/websocket.ts';

export interface UpdateStrip {
  sid?: string
  eobt?: string;
  route?: string
  heading?: number;
  altitude?: number;
  stand?: string;
}

// Define the state interface for our store
export interface WebSocketState {
  controllers: FrontendController[];
  strips: FrontendStrip[];
  position: string;
  airport: string;
  callsign: string;
  runwaySetup: RunwayConfiguration;
  isInitialized: boolean;

  // actions
  move: (callsign: string, bay: Bay) => void;
  generateSquawk: (callsign: string) => void;
  updateStrip(callsign: string, update: UpdateStrip): void;
}

// Create the store using createVanilla
export const createWebSocketStore = (wsClient: WebSocketClient) => {
  // Initial state
  const initialState = {
    controllers: [],
    strips: [],
    position: '',
    airport: '',
    callsign: '',
    runwaySetup: {
      departure: [],
      arrival: []
    },
    isInitialized: false,
  };

  // Create the store
  const store = createStore<WebSocketState>()((set) => ({
    ...initialState,
    move: (callsign, bay) => set((state) => {
        wsClient.send({type: ActionType.FrontendMove, callsign, bay})

        return produce((state: WebSocketState) => {
          const stripIndex = state.strips.findIndex(strip => strip.callsign === callsign);
          if (stripIndex !== -1) {
            state.strips[stripIndex].bay = bay;
          }
          return state;
        })(state)
      }
    ),
    generateSquawk: (callsign) => {
        wsClient.send({type: ActionType.FrontendGenerateSquawk, callsign})
    },
    updateStrip(callsign: string, update: UpdateStrip) {
      wsClient.send({
        type: ActionType.FrontendUpdateStripData,
        callsign,
        eobt: update.eobt,
        route: update.route,
        sid: update.sid,
        heading: update.heading,
        altitude: update.altitude,
        stand: update.stand,
      })

      return produce((state: WebSocketState) => {
        const stripIndex = state.strips.findIndex(strip => strip.callsign === callsign);
        if (stripIndex !== -1) {
          if (update.sid) {
            state.strips[stripIndex].sid = update.sid;
          }
          if (update.eobt) {
            state.strips[stripIndex].eobt = update.eobt;
          }
          if (update.route) {
            state.strips[stripIndex].route = update.route;
          }
          if (update.heading) {
            state.strips[stripIndex].heading = update.heading;
          }
          if (update.altitude) {
            state.strips[stripIndex].cleared_altitude = update.altitude;
          }
          if (update.stand) {
            state.strips[stripIndex].stand = update.stand;
          }
        }
      })
    }
  }));

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

  const handleDisconnectEvent = () => {
    store.setState({...initialState})
  }

  const handleAircraftDisconnectEvent = (data: FrontendAircraftDisconnectEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        state.strips = state.strips.filter(strip => strip.callsign !== data.callsign);
      })
    )
  };

  const handleStandEvent = (data: FrontendStandEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        const stripIndex = state.strips.findIndex(strip => strip.callsign === data.callsign);
        if (stripIndex !== -1) {
          state.strips[stripIndex].stand = data.stand;
        }
      })
    )
  };

  const handleSetHeadingEvent = (data: FrontendSetHeadingEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        const stripIndex = state.strips.findIndex(strip => strip.callsign === data.callsign);
        if (stripIndex !== -1) {
          state.strips[stripIndex].heading = data.heading;
        }
      })
    )
  }

  const handleCommunicationTypeEvent = (data: FrontendCommunicationTypeEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        const stripIndex = state.strips.findIndex(strip => strip.callsign === data.callsign);
        if (stripIndex !== -1) {
          state.strips[stripIndex].communication_type = data.communication_type;
        }
      })
    )
  }


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
  wsClient.on(EventType.FrontendDisconnect, handleDisconnectEvent);
  wsClient.on(EventType.FrontendAircraftDisconnect, handleAircraftDisconnectEvent);
  wsClient.on(EventType.FrontendStand, handleStandEvent);
  wsClient.on(EventType.FrontendSetHeading, handleSetHeadingEvent);
  wsClient.on(EventType.FrontendCommunicationType, handleCommunicationTypeEvent);

  return store;
};
