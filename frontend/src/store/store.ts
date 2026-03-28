import {createStore} from 'zustand/vanilla';
import {produce} from 'immer';
import {
  ActionType,
  Bay,
  EventType,
  type ActionRejectedEvent,
  type FrontendAircraftDisconnectEvent,
  type FrontendAssignedSquawkEvent,
  type FrontendBayEvent, type FrontendBroadcastEvent, type FrontendBulkBayEvent, type FrontendCdmDataEvent, type FrontendCdmWaitEvent,
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
  type FrontendCoordinationTagRequestBroadcastEvent,
  type FrontendRunwayConfigurationEvent,
  type FrontendRequestedAltitudeEvent,
  type FrontendSetHeadingEvent,
  type FrontendSquawkEvent,
  type FrontendStandEvent,
  type FrontendStrip,
  type FrontendStripUpdateEvent,
  type MessageReceived,
  type FrontendMessageReceivedEvent,
  type RunwayConfiguration,
  type StripRef,
  type TacticalStrip,
  type TacticalStripType,
  type FrontendTacticalStripCreatedEvent,
  type FrontendTacticalStripDeletedEvent,
  type FrontendTacticalStripUpdatedEvent,
  type FrontendTacticalStripMovedEvent,
  type FrontendAtisUpdateEvent,
  type SidInfo,
} from '../api/models.ts';
import {WebSocketClient} from '../api/websocket.ts';

const KNOWN_LAYOUTS = new Set(["CLX", "AAAD", "AA", "AD", "ESET", "GEGW", "TWTE"]);

export interface UpdateStrip {
  sid?: string
  eobt?: string;
  route?: string
  heading?: number;
  altitude?: number;
  stand?: string;
  ob?: boolean;
}

export interface BroadcastNotification {
  message: string;
  from: string;
  receivedAt: number; // Unix timestamp in milliseconds from Date.now()
}

// Define the state interface for our store
export interface WebSocketState {
  controllers: FrontendController[];
  strips: FrontendStrip[];
  tacticalStrips: TacticalStrip[];
  position: string;
  identifier: string;
  airport: string;
  callsign: string;
  layout: string;
  displayedLayout: string;
  followRecommendedLayout: boolean;
  layoutChooserOpen: boolean;
  runwaySetup: RunwayConfiguration;
  isInitialized: boolean;
  stripTransfers: Record<string, { from: string; to: string; isTagRequest: boolean }>;

  messages: MessageReceived[];
  broadcastNotifications: BroadcastNotification[];
  metar: string;
  arrAtisCode: string;
  depAtisCode: string;

  availableSids: SidInfo[];

  selectedCallsign: string | null;
  selectStrip: (callsign: string | null) => void;
  setDisplayedLayout: (layout: string) => void;
  setLayoutChooserOpen: (open: boolean) => void;

  contextMenu: { callsign: string; x: number; y: number } | null;
  openStripContextMenu: (callsign: string, pos: { x: number; y: number }) => void;
  closeStripContextMenu: () => void;

  // actions
  move: (callsign: string, bay: Bay) => void;
  generateSquawk: (callsign: string) => void;
  updateOrder: (callsign: string, insertAfter: StripRef | null) => void;
  sendMessage: (text: string, recipients: string[]) => void;
  dismissMessage: (id: number) => void;
  updateStrip: (callsign: string, update: UpdateStrip) => void;
  setReleasePoint: (callsign: string, releasePoint: string) => void;
  issuePdcClearance: (callsign: string, remarks: string | null) => void;
  revertToVoice: (callsign: string) => void;
  transferStrip: (callsign: string, toPosition: string) => void;
  assumeStrip: (callsign: string) => void;
  forceAssumeStrip: (callsign: string) => void;
  freeStrip: (callsign: string) => void;
  cancelTransfer: (callsign: string) => void;
  requestTag: (callsign: string) => void;
  acceptTagRequest: (callsign: string) => void;
  toggleMarked: (callsign: string, marked: boolean) => void;
  runwayClearance: (callsign: string) => void;
  runwayConfirmation: (callsign: string) => void;
  cdmReady: (callsign: string) => void;
  assignRunway: (callsign: string, runway: string) => void;

  acknowledgeUnexpectedChange: (callsign: string, fieldName: string) => void;

  missedApproach: (callsign: string) => void;
  updateRunwayStatus: (pair: string, status: string) => void;

  // manual FPL actions
  createManualFPL: (callsign: string, ades: string, sid: string, ssr: string, eobt: string, aircraftType: string, fl: string, route: string, stand: string, rwyDep: string) => void;
  createVFRFPL: (callsign: string, aircraftType: string, personsOnBoard: number, ssr: string, fplType: string, language: string, remarks: string) => void;

  // tactical strip actions
  createTacticalStrip: (stripType: TacticalStripType, bay: string, label: string, aircraft: string) => void;
  deleteTacticalStrip: (id: number) => void;
  confirmTacticalStrip: (id: number) => void;
  startTacticalTimer: (id: number) => void;
  moveTacticalStrip: (id: number, insertAfter: StripRef | null) => void;
}

// Create the store using createVanilla
export const createWebSocketStore = (wsClient: WebSocketClient) => {
  // Initial state
  const initialState = {
    controllers: [],
    strips: [],
    tacticalStrips: [],
    position: '',
    identifier: '',
    airport: '',
    callsign: '',
    layout: '',
    displayedLayout: '',
    followRecommendedLayout: true,
    layoutChooserOpen: false,
    runwaySetup: {
      departure: [],
      arrival: []
    },
    isInitialized: false,
    stripTransfers: {},
    messages: [],
    broadcastNotifications: [],
    metar: "",
    arrAtisCode: "",
    depAtisCode: "",
    availableSids: [],
    selectedCallsign: null,
    contextMenu: null
  };

  // Create the store
  const store = createStore<WebSocketState>()((set, get) => ({
    ...initialState,
    selectStrip: (callsign) => set({ selectedCallsign: callsign }),
    setDisplayedLayout: (layout) => set({
      displayedLayout: layout,
      followRecommendedLayout: layout === get().layout,
    }),
    setLayoutChooserOpen: (open) => set({ layoutChooserOpen: open }),
    openStripContextMenu: (callsign, pos) => set({ contextMenu: { callsign, x: pos.x, y: pos.y } }),
    closeStripContextMenu: () => set({ contextMenu: null }),
    move: (callsign, bay) => set((state) => {
        wsClient.send({type: ActionType.FrontendMove, callsign, bay})

        return produce((state: WebSocketState) => {
          const stripIndex = state.strips.findIndex(strip => strip.callsign === callsign);
          if (stripIndex !== -1) {
            state.strips[stripIndex].bay = bay;
            // Optimistically assign end-of-bay sequence matching backend (max of flights+tacticals + spacing)
            const maxFlight = state.strips
              .filter(s => s.bay === bay && s.callsign !== callsign)
              .reduce((m, s) => Math.max(m, s.sequence), 0);
            const maxTactical = state.tacticalStrips
              .filter(t => t.bay === bay)
              .reduce((m, t) => Math.max(m, t.sequence), 0);
            state.strips[stripIndex].sequence = Math.max(maxFlight, maxTactical) + 1000;
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
        ob: update.ob,
      })

      set((state) =>
        produce(state, (draft: WebSocketState) => {
          const stripIndex = draft.strips.findIndex(strip => strip.callsign === callsign);
          if (stripIndex !== -1) {
            if (update.sid !== undefined) {
              draft.strips[stripIndex].sid = update.sid;
            }
            if (update.eobt !== undefined) {
              draft.strips[stripIndex].eobt = update.eobt;
            }
            if (update.route !== undefined) {
              draft.strips[stripIndex].route = update.route;
            }
            if ("heading" in update && update.heading !== undefined) {
              draft.strips[stripIndex].heading = update.heading;
            }
            if ("altitude" in update && update.altitude !== undefined) {
              draft.strips[stripIndex].cleared_altitude = update.altitude;
            }
            if (update.stand !== undefined) {
              draft.strips[stripIndex].stand = update.stand;
            }
            if (update.ob !== undefined) {
              draft.strips[stripIndex].ob = update.ob;
            }
          }
        })
      );
    },
    updateOrder: (callsign, insertAfter) => set((state) => {
      wsClient.send({type: ActionType.FrontendUpdateOrder, callsign: callsign, insert_after: insertAfter})

      return produce((draft: WebSocketState) => {
        // Optimistically update sequence using the same midpoint formula as the backend
        const stripIndex = draft.strips.findIndex(strip => strip.callsign === callsign)
        if (stripIndex === -1) return;

        const strip = draft.strips[stripIndex];
        const bay = strip.bay;

        // All sequences in the bay except the strip being moved, sorted ascending
        const baySeqs = [
          ...draft.strips.filter(s => s.bay === bay && s.callsign !== strip.callsign).map(s => s.sequence),
          ...draft.tacticalStrips.filter(t => t.bay === bay).map(t => t.sequence),
        ].sort((a, b) => a - b);

        let prevSeq: number;
        let nextSeq: number | null;

        if (insertAfter === null) {
          prevSeq = 0;
          nextSeq = baySeqs[0] ?? null;
        } else if (insertAfter.kind === 'flight' && insertAfter.callsign) {
          const afterStrip = draft.strips.find(s => s.callsign === insertAfter.callsign);
          if (!afterStrip) return;
          prevSeq = afterStrip.sequence;
          const afterIdx = baySeqs.indexOf(prevSeq);
          nextSeq = baySeqs[afterIdx + 1] ?? null;
        } else if (insertAfter.kind === 'tactical' && insertAfter.id !== undefined) {
          const afterTactical = draft.tacticalStrips.find(t => t.id === insertAfter.id);
          if (!afterTactical) return;
          prevSeq = afterTactical.sequence;
          const afterIdx = baySeqs.indexOf(prevSeq);
          nextSeq = baySeqs[afterIdx + 1] ?? null;
        } else {
          return;
        }

        draft.strips[stripIndex].sequence = nextSeq === null
          ? prevSeq + 100
          : Math.floor((prevSeq + nextSeq) / 2);
      })(state)
    }),
    sendMessage: (text, recipients) => {
      wsClient.send({type: ActionType.FrontendSendMessage, text, recipients})
    },
    dismissMessage: (id) => {
      store.setState(
        produce((state: WebSocketState) => {
          state.messages = state.messages.filter(m => m.id !== id);
        })
      );
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
    // forceAssumeStrip: takes ownership of an unowned strip, bypassing the next-owners check
    forceAssumeStrip: (callsign) => {
      wsClient.send({ type: ActionType.FrontendCoordinationForceAssumeRequest, callsign });
    },
    freeStrip: (callsign) => {
      wsClient.send({ type: ActionType.FrontendCoordinationFreeRequest, callsign });
    },
    cancelTransfer: (callsign) => {
      wsClient.send({ type: ActionType.FrontendCoordinationCancelTransferRequest, callsign });
    },
    requestTag: (callsign) => {
      wsClient.send({ type: ActionType.FrontendCoordinationTagRequest, callsign });
    },
    acceptTagRequest: (callsign) => {
      wsClient.send({ type: ActionType.FrontendCoordinationAcceptTagRequest, callsign });
    },
    cdmReady: (callsign) => {
      wsClient.send({ type: ActionType.FrontendCdmReady, callsign });
    },
    assignRunway: (callsign, runway) => {
      wsClient.send({ type: ActionType.FrontendUpdateStripData, callsign, runway });
      store.setState(
        produce((state: WebSocketState) => {
          const idx = state.strips.findIndex(s => s.callsign === callsign);
          if (idx !== -1) state.strips[idx].runway = runway;
        })
      );
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
    runwayClearance: (callsign) => {
      wsClient.send({ type: ActionType.FrontendRunwayClearance, callsign });
      store.setState(
        produce((state: WebSocketState) => {
          const idx = state.strips.findIndex(s => s.callsign === callsign);
          if (idx !== -1) {
            // Auto-confirm if no other strips are already confirmed in the session.
            const hasConfirmed = state.strips.some(s => s.callsign !== callsign && s.runway_confirmed);
            state.strips[idx].runway_cleared = true;
            state.strips[idx].runway_confirmed = !hasConfirmed;
            if (state.strips[idx].bay === Bay.TaxiLwr) state.strips[idx].bay = Bay.Depart;
            if (state.strips[idx].bay === Bay.Final) state.strips[idx].bay = Bay.RwyArr;
          }
        })
      );
    },
    runwayConfirmation: (callsign) => {
      wsClient.send({ type: ActionType.FrontendRunwayConfirmation, callsign });
      store.setState(
        produce((state: WebSocketState) => {
          const idx = state.strips.findIndex(s => s.callsign === callsign);
          if (idx !== -1) {
            state.strips[idx].runway_confirmed = true;
          }
        })
      );
    },
    acknowledgeUnexpectedChange: (callsign, fieldName) => {
      wsClient.send({ type: ActionType.FrontendAcknowledgeUnexpectedChange, callsign, field_name: fieldName });
      store.setState(
        produce((state: WebSocketState) => {
          const idx = state.strips.findIndex(s => s.callsign === callsign);
          if (idx !== -1) {
            state.strips[idx].unexpected_change_fields = (state.strips[idx].unexpected_change_fields ?? []).filter(f => f !== fieldName);
          }
        })
      );
    },
    missedApproach: (callsign) => {
      wsClient.send({ type: ActionType.FrontendMissedApproach, callsign });
    },
    updateRunwayStatus: (pair, status) => {
      wsClient.send({ type: ActionType.FrontendUpdateRunwayStatus, pair, status });
    },
    createManualFPL: (callsign, ades, sid, ssr, eobt, aircraftType, fl, route, stand, rwyDep) => {
      wsClient.send({ type: ActionType.FrontendCreateManualFPL, callsign, ades, sid, ssr, eobt, aircraft_type: aircraftType, fl, route, stand, rwy_dep: rwyDep });
    },
    createVFRFPL: (callsign, aircraftType, personsOnBoard, ssr, fplType, language, remarks) => {
      wsClient.send({ type: ActionType.FrontendCreateVFRFPL, callsign, aircraft_type: aircraftType, persons_on_board: personsOnBoard, ssr, fpl_type: fplType, language, remarks });
    },
    createTacticalStrip:(stripType, bay, label, aircraft) => {
      wsClient.send({ type: ActionType.FrontendCreateTacticalStrip, strip_type: stripType, bay, label, aircraft });
    },
    deleteTacticalStrip: (id) => {
      wsClient.send({ type: ActionType.FrontendDeleteTacticalStrip, id });
    },
    confirmTacticalStrip: (id) => {
      wsClient.send({ type: ActionType.FrontendConfirmTacticalStrip, id });
    },
    startTacticalTimer: (id) => {
      wsClient.send({ type: ActionType.FrontendStartTacticalTimer, id });
    },
    moveTacticalStrip: (id, insertAfter) => set((state) => {
      wsClient.send({ type: ActionType.FrontendMoveTacticalStrip, id, insert_after: insertAfter });

      return produce((draft: WebSocketState) => {
        const idx = draft.tacticalStrips.findIndex(t => t.id === id);
        if (idx === -1) return;

        const bay = draft.tacticalStrips[idx].bay;

        // All sequences in the bay except the strip being moved, sorted ascending
        const baySeqs = [
          ...draft.strips.filter(s => s.bay === bay).map(s => s.sequence),
          ...draft.tacticalStrips.filter(t => t.bay === bay && t.id !== id).map(t => t.sequence),
        ].sort((a, b) => a - b);

        let prevSeq: number;
        let nextSeq: number | null;

        if (insertAfter === null) {
          prevSeq = 0;
          nextSeq = baySeqs[0] ?? null;
        } else if (insertAfter.kind === 'flight' && insertAfter.callsign) {
          const afterStrip = draft.strips.find(s => s.callsign === insertAfter.callsign);
          if (!afterStrip) return;
          prevSeq = afterStrip.sequence;
          const afterIdx = baySeqs.indexOf(prevSeq);
          nextSeq = baySeqs[afterIdx + 1] ?? null;
        } else if (insertAfter.kind === 'tactical' && insertAfter.id !== undefined) {
          const afterTactical = draft.tacticalStrips.find(t => t.id === insertAfter.id);
          if (!afterTactical) return;
          prevSeq = afterTactical.sequence;
          const afterIdx = baySeqs.indexOf(prevSeq);
          nextSeq = baySeqs[afterIdx + 1] ?? null;
        } else {
          return;
        }

        draft.tacticalStrips[idx].sequence = nextSeq === null
          ? prevSeq + 100
          : Math.floor((prevSeq + nextSeq) / 2);
      })(state);
    }),
  }));

  // Private methods to handle WebSocket events
  const handleInitialEvent = (data: FrontendInitialEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        state.controllers = data.controllers;
        state.strips = data.strips;
        state.tacticalStrips = data.tactical_strips ?? [];
        state.position = data.me.position;
        state.identifier = data.me.identifier;
        state.airport = data.airport;
        state.callsign = data.callsign;
        state.layout = data.layout;
        if (KNOWN_LAYOUTS.has(data.layout)) {
          state.displayedLayout = data.layout;
          state.followRecommendedLayout = true;
        } else {
          state.displayedLayout = "";
          state.followRecommendedLayout = true;
        }
        state.runwaySetup = data.runway_setup;
        state.isInitialized = true;
        const transfers: Record<string, { from: string; to: string; isTagRequest: boolean }> = {};
        for (const coord of data.coordinations) {
          transfers[coord.callsign] = { from: coord.from, to: coord.to, isTagRequest: coord.is_tag_request };
        }
        state.stripTransfers = transfers;
        state.messages = data.messages ?? [];
        state.availableSids = data.available_sids ?? [];
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
            section: data.section,
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

  // Handle all sequence updates for a bay in a single setState call to prevent
  // transient ordering inconsistencies when strips are recalculated in bulk.
  const handleBulkBayEvent = (data: FrontendBulkBayEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        for (const entry of data.strips) {
          const stripIndex = state.strips.findIndex(s => s.callsign === entry.callsign);
          if (stripIndex !== -1) {
            state.strips[stripIndex].bay = data.bay;
            state.strips[stripIndex].sequence = entry.sequence;
          }
          if (state.selectedCallsign === entry.callsign) {
            state.selectedCallsign = null;
          }
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
          state.strips[stripIndex].owner = data.owner;
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
        if (KNOWN_LAYOUTS.has(data.layout)) {
          if (state.followRecommendedLayout) {
            state.displayedLayout = data.layout;
          }
        } else {
          if (state.followRecommendedLayout) {
            state.displayedLayout = "";
          }
        }
      })
    )
  }

  const handleBroadcastEvent = (data: FrontendBroadcastEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        state.broadcastNotifications = [
          {
            message: data.message,
            from: data.from,
            receivedAt: Date.now(),
          },
          ...state.broadcastNotifications,
        ].slice(0, 50);

        // Also push into the messages panel so it is visible in the view.
        state.messages = [
          {
            id: Date.now(),
            sender: data.from,
            text: data.message,
            is_broadcast: true,
            recipients: [],
          },
          ...state.messages,
        ].slice(0, 100);
      })
    );
  }

  const handleMessageReceived = (data: FrontendMessageReceivedEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        state.messages = [data, ...state.messages].slice(0, 100);
      })
    );
  };

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
        state.stripTransfers[data.callsign] = { from: data.from, to: data.to, isTagRequest: false };
      })
    );
  };

  const handleCoordinationTagRequestBroadcastEvent = (data: FrontendCoordinationTagRequestBroadcastEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        state.stripTransfers[data.callsign] = { from: data.from, to: data.to, isTagRequest: true };
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

  const handleRunwayConfigurationEvent = (data: FrontendRunwayConfigurationEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        state.runwaySetup = data.runway_setup;
      })
    );
  };

  const handleTacticalStripCreatedEvent = (data: FrontendTacticalStripCreatedEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        state.tacticalStrips.push(data.strip);
      })
    );
  };

  const handleTacticalStripDeletedEvent = (data: FrontendTacticalStripDeletedEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        state.tacticalStrips = state.tacticalStrips.filter(ts => ts.id !== data.id);
      })
    );
  };

  const handleTacticalStripUpdatedEvent = (data: FrontendTacticalStripUpdatedEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        const idx = state.tacticalStrips.findIndex(ts => ts.id === data.strip.id);
        if (idx !== -1) {
          state.tacticalStrips[idx] = data.strip;
        }
      })
    );
  };

  const handleTacticalStripMovedEvent = (data: FrontendTacticalStripMovedEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        const idx = state.tacticalStrips.findIndex(ts => ts.id === data.id);
        if (idx !== -1) {
          state.tacticalStrips[idx].bay = data.bay;
          state.tacticalStrips[idx].sequence = data.sequence;
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
  wsClient.on(EventType.FrontendBulkBay, handleBulkBayEvent);
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
  wsClient.on(EventType.FrontendCoordinationTagRequestBroadcast, handleCoordinationTagRequestBroadcastEvent);
  wsClient.on(EventType.FrontendRunWayConfiguration, handleRunwayConfigurationEvent);
  wsClient.on(EventType.FrontendTacticalStripCreated, handleTacticalStripCreatedEvent);
  wsClient.on(EventType.FrontendTacticalStripDeleted, handleTacticalStripDeletedEvent);
  wsClient.on(EventType.FrontendTacticalStripUpdated, handleTacticalStripUpdatedEvent);
  wsClient.on(EventType.FrontendTacticalStripMoved, handleTacticalStripMovedEvent);
  wsClient.on(EventType.FrontendMessageReceived, handleMessageReceived);

  const handleAtisUpdateEvent = (data: FrontendAtisUpdateEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        state.metar = data.metar;
        state.arrAtisCode = data.arr_atis_code;
        state.depAtisCode = data.dep_atis_code;
      })
    );
  };

  wsClient.on(EventType.FrontendAtisUpdate, handleAtisUpdateEvent);

  const handleActionRejectedEvent = (_data: ActionRejectedEvent) => {
    // Reconnect to receive a fresh initial event from the server,
    // which overwrites any optimistic updates that were rejected.
    wsClient.reconnect();
  };

  wsClient.on(EventType.FrontendActionRejected, handleActionRejectedEvent);

  wsClient.on(EventType.FrontendAvailableSids, (data) => {
    store.setState({ availableSids: data.sids });
  });

  return store;
};
