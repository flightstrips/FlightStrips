import {createStore} from 'zustand/vanilla';
import {produce} from 'immer';
import {
  ActionType,
  Bay,
  EventType,
  type FrontendAircraftDisconnectEvent,
  type FrontendAssignedSquawkEvent,
  type FrontendBayEvent, type FrontendBroadcastEvent, type FrontendCdmDataEvent, type FrontendCdmWaitEvent,
  type FrontendClearedAltitudeEvent,
  type FrontendCommunicationTypeEvent,
  type FrontendController,
  type FrontendControllerOfflineEvent,
  type FrontendControllerOnlineEvent,
  type FrontendInitialEvent,
  type FrontendLayoutUpdateEvent,
  type FrontendOwnersUpdateEvent, type FrontendPdcStateUpdateEvent, type FrontendReleasePointEvent,
  type FrontendMarkedEvent,
  type FrontendCoordinationTransferBroadcastEvent,
  type FrontendCoordinationAssumeBroadcastEvent,
  type FrontendCoordinationRejectBroadcastEvent,
  type FrontendCoordinationFreeBroadcastEvent,
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
  identifier: string;
  airport: string;
  callsign: string;
  layout: string;
  runwaySetup: RunwayConfiguration;
  isInitialized: boolean;
  stripTransfers: Record<string, string>;

  activeMessages: FrontendBroadcastEvent[];

  selectedCallsign: string | null;
  selectStrip: (callsign: string | null) => void;

  // actions
  move: (callsign: string, bay: Bay) => void;
  generateSquawk: (callsign: string) => void;
  updateOrder: (callsign: string, before: string | null) => void;
  sendMessage: (message: string, to: string | null) => void;
  updateStrip: (callsign: string, update: UpdateStrip) => void;
  setReleasePoint: (callsign: string, releasePoint: string) => void;
  issuePdcClearance: (callsign: string, remarks: string | null) => void;
  revertToVoice: (callsign: string) => void;
  transferStrip: (callsign: string, toPosition: string) => void;
  assumeStrip: (callsign: string) => void;
  freeStrip: (callsign: string) => void;
  toggleMarked: (callsign: string, marked: boolean) => void;
}

// Create the store using createVanilla
export const createWebSocketStore = (wsClient: WebSocketClient) => {
  // Initial state
  const initialState = {
    controllers: [],
    strips: [],
    position: '',
    identifier: '',
    airport: '',
    callsign: '',
    layout: '',
    runwaySetup: {
      departure: [],
      arrival: []
    },
    isInitialized: false,
    stripTransfers: {},
    activeMessages: [],
    selectedCallsign: null
  };

  // Create the store
  const store = createStore<WebSocketState>()((set) => ({
    ...initialState,
    selectStrip: (callsign) => set({ selectedCallsign: callsign }),
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
    },
    updateOrder: (callsign, before) => set((state) => {
      wsClient.send({type: ActionType.FrontendUpdateOrder, callsign: callsign, before: before})

      return produce((draft: WebSocketState) => {
        // Optimistically update sequence so the UI reflects the new order immediately
        const stripIndex = draft.strips.findIndex(strip => strip.callsign === callsign)
        if (stripIndex === -1) return;

        if (before === null) {
          // Append to end
          draft.strips[stripIndex].sequence = -1;
        } else {
          const beforeIndex = draft.strips.findIndex(strip => strip.callsign === before)
          if (beforeIndex === -1) return;
          draft.strips[stripIndex].sequence = draft.strips[beforeIndex].sequence + 1
        }
      })(state)
    }),
    sendMessage: (message, to) => {
      wsClient.send({type: ActionType.FrontendSendMessage, message, to})
    },
    setReleasePoint: (callsign, releasePoint) => {
      wsClient.send({type: ActionType.FrontendReleasePoint, callsign: callsign, release_point: releasePoint})

      return produce((state: WebSocketState) => {
        const stripIndex = state.strips.findIndex(strip => strip.callsign === callsign)
        if (stripIndex !== -1) {
          state.strips[stripIndex].release_point = releasePoint
        }
      })
    },
    issuePdcClearance: (callsign, remarks) => {
      wsClient.send({type: ActionType.FrontendIssuePdcClearanceRequest, callsign, remarks})

      return produce((state: WebSocketState) => {
        const stripIndex = state.strips.findIndex(strip => strip.callsign === callsign)
        if (stripIndex !== -1) {
          state.strips[stripIndex].pdc_state = "CLEARED"
        }
      })
    },
    revertToVoice: (callsign) => {
      wsClient.send({type: ActionType.FrontendRevertToVoiceRequest, callsign})

      return produce((state: WebSocketState) => {
        const stripIndex = state.strips.findIndex(strip => strip.callsign === callsign)
        if (stripIndex !== -1) {
          state.strips[stripIndex].pdc_state = "REVERT_TO_VOICE"
        }
      })
    },
    transferStrip: (callsign, toPosition) => {
      wsClient.send({
        type: ActionType.FrontendCoordinationTransferRequest,
        callsign,
        to: toPosition,
      });
    },
    assumeStrip: (callsign) => {
      wsClient.send({ type: ActionType.FrontendCoordinationAssumeRequest, callsign });
    },
    freeStrip: (callsign) => {
      wsClient.send({ type: ActionType.FrontendCoordinationFreeRequest, callsign });
    },
    toggleMarked: (callsign, marked) => {
      wsClient.send({ type: ActionType.FrontendMarked, callsign, marked });
      store.setState(
        produce((state: WebSocketState) => {
          const idx = state.strips.findIndex(s => s.callsign === callsign);
          if (idx !== -1) state.strips[idx].marked = marked;
        })
      );
    },
  }));

  // Private methods to handle WebSocket events
  const handleInitialEvent = (data: FrontendInitialEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        state.controllers = data.controllers;
        state.strips = data.strips;
        state.position = data.me.position;
        state.identifier = data.me.identifier;
        state.airport = data.airport;
        state.callsign = data.callsign;
        state.layout = data.layout;
        state.runwaySetup = data.runway_setup;
        state.isInitialized = true;
        const transfers: Record<string, string> = {};
        for (const coord of data.coordinations) {
          transfers[coord.callsign] = coord.to;
        }
        state.stripTransfers = transfers;
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
            position: data.position,
            identifier: data.identifier,
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
          state.strips[stripIndex].sequence = data.sequence;
        }
        if (state.selectedCallsign === data.callsign) {
          state.selectedCallsign = null;
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

  const handleOwnersUpdateEvent = (data: FrontendOwnersUpdateEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        const stripIndex = state.strips.findIndex(strip => strip.callsign === data.callsign);
        if (stripIndex !== -1) {
          state.strips[stripIndex].next_controllers = data.next_owners;
          state.strips[stripIndex].previous_controllers = data.previous_owners;
        }
      })
    )
  }

  const handleLayoutUpdateEvent = (data: FrontendLayoutUpdateEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        state.layout = data.layout;
      })
    )
  }

  const handleBroadcastEvent = (data: FrontendBroadcastEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        state.activeMessages.push(data);
      })
    )
  }

  const handleCdmUpdateEvent = (data: FrontendCdmDataEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        const stripIndex = state.strips.findIndex(strip => strip.callsign === data.callsign);
        if (stripIndex !== -1) {
          state.strips[stripIndex].tobt = data.tobt
          state.strips[stripIndex].eobt = data.eobt
          state.strips[stripIndex].tsat = data.tsat
          state.strips[stripIndex].ctot = data.ctot
        }
      })
    )
  }

  const handleCdmWaitEvent = (_data: FrontendCdmWaitEvent) => {
    // TODO set marker on strip to indicate that we are waiting for CDM data
    // this is the case when a strip requests a new tobt
  }

  const handleReleasePointEvent = (data: FrontendReleasePointEvent) => {
    store.setState(
        produce((state: WebSocketState) => {
          const stripIndex = state.strips.findIndex(strip => strip.callsign == data.callsign);
          if (stripIndex !== -1) {
            state.strips[stripIndex].release_point = data.release_point;
          }
        })
    )
  }

  const handleMarkedEvent = (data: FrontendMarkedEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        const idx = state.strips.findIndex(s => s.callsign === data.callsign);
        if (idx !== -1) state.strips[idx].marked = data.marked;
      })
    );
  };

  const handlePdcStateUpdateEvent = (data: FrontendPdcStateUpdateEvent) => {
    store.setState(
        produce((state: WebSocketState) => {
          const stripIndex = state.strips.findIndex(strip => strip.callsign == data.callsign);
          if (stripIndex !== -1) {
            state.strips[stripIndex].pdc_state = data.state;
          }
        })
    )
  }

  const handleCoordinationTransferBroadcastEvent = (data: FrontendCoordinationTransferBroadcastEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        state.stripTransfers[data.callsign] = data.to;
      })
    );
  };

  const handleCoordinationAssumeBroadcastEvent = (data: FrontendCoordinationAssumeBroadcastEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        delete state.stripTransfers[data.callsign];
      })
    );
  };

  const handleCoordinationRejectBroadcastEvent = (data: FrontendCoordinationRejectBroadcastEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        delete state.stripTransfers[data.callsign];
      })
    );
  };

  const handleCoordinationFreeBroadcastEvent = (data: FrontendCoordinationFreeBroadcastEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        delete state.stripTransfers[data.callsign];
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
  wsClient.on(EventType.FrontendDisconnect, handleDisconnectEvent);
  wsClient.on(EventType.FrontendAircraftDisconnect, handleAircraftDisconnectEvent);
  wsClient.on(EventType.FrontendStand, handleStandEvent);
  wsClient.on(EventType.FrontendSetHeading, handleSetHeadingEvent);
  wsClient.on(EventType.FrontendCommunicationType, handleCommunicationTypeEvent);
  wsClient.on(EventType.FrontendOwnersUpdate, handleOwnersUpdateEvent);
  wsClient.on(EventType.FrontendLayoutUpdate, handleLayoutUpdateEvent);
  wsClient.on(EventType.FrontendBroadcast, handleBroadcastEvent);
  wsClient.on(EventType.FrontendCdmData, handleCdmUpdateEvent);
  wsClient.on(EventType.FrontendCdmWait, handleCdmWaitEvent);
  wsClient.on(EventType.FrontendReleasePoint, handleReleasePointEvent);
  wsClient.on(EventType.FrontendMarked, handleMarkedEvent);
  wsClient.on(EventType.FrontendPdcStateChange, handlePdcStateUpdateEvent);
  wsClient.on(EventType.FrontendCoordinationTransferBroadcast, handleCoordinationTransferBroadcastEvent);
  wsClient.on(EventType.FrontendCoordinationAssumeBroadcast, handleCoordinationAssumeBroadcastEvent);
  wsClient.on(EventType.FrontendCoordinationRejectBroadcast, handleCoordinationRejectBroadcastEvent);
  wsClient.on(EventType.FrontendCoordinationFreeBroadcast, handleCoordinationFreeBroadcastEvent);

  return store;
};
