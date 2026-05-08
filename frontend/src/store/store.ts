import {createStore} from 'zustand/vanilla';
import {produce} from 'immer';
import {
  ActionType,
  Bay,
  EventType,
  type FrontendAircraftDisconnectEvent,
  type FrontendAssignedSquawkEvent,
  type FrontendBayEvent, type FrontendBroadcastEvent, type FrontendBulkBayEvent, type FrontendCdmDataEvent, type FrontendCdmWaitEvent,
  type FrontendClearedAltitudeEvent,
  type FrontendCommunicationTypeEvent,
  type FrontendController,
  type FrontendControllerOfflineEvent,
  type FrontendControllerOnlineEvent,
  type FrontendControllerUpdateEvent,
  type FrontendDisconnectEvent,
  type FrontendGoAroundEvent,
  type FrontendInitialEvent,
  type FrontendLayoutUpdateEvent,
  type FrontendOwnersUpdateEvent, type FrontendPdcStateUpdateEvent, type FrontendReleasePointEvent,
  type FrontendMarkedEvent,
  type FrontendSendEvent,
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
import missedApproachSound from "@/assets/missed_approach.mp3";
import { isAudioMuted } from "@/lib/audio-settings";
import {
  isValidationActionAllowed,
  isValidationActiveForPosition,
  type ValidationAttemptedAction,
  type ValidationEditableField,
} from "@/lib/validation-status";
import { normalizeCdmTime } from "@/lib/cdmTime";
import { toast } from "sonner";

const KNOWN_LAYOUTS = new Set(["CLX", "AAAD", "AA", "AD", "EST", "GEGW", "TWTE", "TWRGND"]);

function normalizeLayout(layout: string) {
  switch (layout.trim().toUpperCase()) {
    case "SEQPLN":
      return "EST";
    case "TETW":
      return "TWTE";
    case "TWR+GND":
      return "TWRGND";
    default:
      return layout.trim().toUpperCase();
  }
}

function nextSequenceAtEndOfBay(strips: FrontendStrip[], tacticalStrips: TacticalStrip[], bay: Bay, movingCallsign?: string): number {
  const maxFlight = strips
    .filter((strip) => strip.bay === bay && strip.callsign !== movingCallsign)
    .reduce((maxSequence, strip) => Math.max(maxSequence, strip.sequence), 0);
  const maxTactical = tacticalStrips
    .filter((strip) => strip.bay === bay)
    .reduce((maxSequence, strip) => Math.max(maxSequence, strip.sequence), 0);

  return Math.max(maxFlight, maxTactical) + 1000;
}

function runwayClearanceTargetBay(bay: string): Bay | null {
  if (bay === Bay.TaxiLwr) return Bay.Depart;
  if (bay === Bay.Final) return Bay.RwyArr;
  return null;
}

function shouldResetRunwayClearanceOnMove(currentBay: string, targetBay: Bay): boolean {
  if (currentBay === Bay.Depart) {
    return targetBay !== Bay.Depart && targetBay !== Bay.Airborne;
  }

  return currentBay === Bay.RwyArr && targetBay === Bay.Final;
}

export interface UpdateStrip {
  sid?: string
  eobt?: string;
  route?: string
  heading?: number;
  altitude?: number;
  stand?: string;
  ob?: boolean;
  remarks?: string;
  aircraft_type?: string;
  capabilities?: string;
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
  readOnly: boolean;
  positionAvailable: boolean;
  followRecommendedLayout: boolean;
  layoutChooserOpen: boolean;
  runwaySetup: RunwayConfiguration;
  initialCflByRunway: Record<string, number>;
  transitionAltitude: number;
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
  tagRequestArmed: boolean;
  setTagRequestArmed: (armed: boolean) => void;
  markArmed: boolean;
  setMarkArmed: (armed: boolean) => void;
  setDisplayedLayout: (layout: string) => void;
  setLayoutChooserOpen: (open: boolean) => void;

  contextMenu: { callsign: string; x: number; y: number } | null;
  openStripContextMenu: (callsign: string, pos: { x: number; y: number }) => void;
  closeStripContextMenu: () => void;
  validationDialogCallsign: string | null;
  openValidationDialog: (callsign: string) => void;
  closeValidationDialog: () => void;

  // actions
  move: (callsign: string, bay: Bay) => void;
  generateSquawk: (callsign: string) => boolean;
  updateOrder: (callsign: string, insertAfter: StripRef | null) => void;
  sendMessage: (text: string, recipients: string[]) => void;
  dismissMessage: (id: number) => void;
  updateStrip: (callsign: string, update: UpdateStrip) => void;
  setReleasePoint: (callsign: string, releasePoint: string) => void;
  setStartReq: (callsign: string, startReq: boolean) => void;
  issuePdcClearance: (callsign: string, remarks: string | null) => void;
  revertToVoice: (callsign: string) => void;
  transferStrip: (callsign: string, toPosition: string) => void;
  assumeStrip: (callsign: string) => void;
  forceAssumeStrip: (callsign: string) => void;
  pickupStrip: (callsign: string, bay: Bay) => void;
  freeStrip: (callsign: string) => void;
  cancelTransfer: (callsign: string) => void;
  requestTag: (callsign: string) => void;
  acceptTagRequest: (callsign: string) => void;
  toggleMarked: (callsign: string, marked: boolean) => void;
  runwayClearance: (callsign: string) => void;
  runwayConfirmation: (callsign: string) => void;
  cdmReady: (callsign: string) => void;
  clxUpdateTobt: (callsign: string) => void;
  clxOverrideValidation: (callsign: string, overrideKey: string) => void;
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
  moveTacticalStrip: (id: number, insertAfter: StripRef | null, bay?: Bay) => void;

  acknowledgeValidationStatus: (callsign: string, activationKey: string) => void;
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
    readOnly: false,
    positionAvailable: true,
    followRecommendedLayout: true,
    layoutChooserOpen: false,
    runwaySetup: {
      departure: [],
      arrival: []
    },
    initialCflByRunway: {},
    transitionAltitude: 0,
    isInitialized: false,
    stripTransfers: {},
    messages: [],
    broadcastNotifications: [],
    metar: "",
    arrAtisCode: "",
    depAtisCode: "",
     availableSids: [],
     selectedCallsign: null,
     tagRequestArmed: false,
     markArmed: false,
     contextMenu: null,
      validationDialogCallsign: null,
      };

  // Create the store
  const store = createStore<WebSocketState>()((set, get) => {
    const ensureWritable = () => !get().readOnly;
    const sendIfWritable = (event: FrontendSendEvent) => {
      if (!ensureWritable()) {
        return false;
      }

      wsClient.send(event);
      return true;
    };

    const findStrip = (callsign: string) => get().strips.find((strip) => strip.callsign === callsign);
    const updateLocalStartReq = (callsign: string, startReq: boolean) => {
      store.setState(
        produce((state: WebSocketState) => {
          const stripIndex = state.strips.findIndex((strip) => strip.callsign === callsign);
          if (stripIndex !== -1) {
            state.strips[stripIndex].start_req = startReq;
          }
        }),
      );
    };
    const openValidationDialog = (callsign: string) => set({ validationDialogCallsign: callsign, contextMenu: null });

    const guardValidationAction = (callsign: string, action: ValidationAttemptedAction) => {
      const strip = findStrip(callsign);
      const myPosition = get().position;

      if (!strip || !isValidationActiveForPosition(strip.validation_status, myPosition)) {
        return true;
      }

      if (isValidationActionAllowed(strip.validation_status, action)) {
        return true;
      }

      openValidationDialog(callsign);
      return false;
    };

    const sendGuardedStripEvent = (
      callsign: string,
      attemptedAction: ValidationAttemptedAction,
      event: FrontendSendEvent,
    ) => {
      if (!guardValidationAction(callsign, attemptedAction)) {
        return false;
      }

      if (!sendIfWritable(event)) {
        return false;
      }

      return true;
    };

    const updateStripFieldsFromPayload = (update: UpdateStrip): ValidationEditableField[] => {
      const fields: ValidationEditableField[] = [];

      if (update.sid !== undefined) fields.push("sid");
      if (update.eobt !== undefined) fields.push("eobt");
      if (update.route !== undefined) fields.push("route");
      if (update.heading !== undefined) fields.push("heading");
      if (update.altitude !== undefined) fields.push("altitude");
      if (update.stand !== undefined) fields.push("stand");
      if (update.remarks !== undefined) fields.push("remarks");
      if (update.aircraft_type !== undefined) fields.push("aircraft_type");
      if (update.capabilities !== undefined) fields.push("capabilities");

      return fields;
    };

    return {
     ...initialState,
     selectStrip: (callsign) => set({ selectedCallsign: callsign }),
     setTagRequestArmed: (armed) => set({ tagRequestArmed: armed, markArmed: armed ? false : get().markArmed, contextMenu: null }),
     setMarkArmed: (armed) => set({ markArmed: armed, tagRequestArmed: armed ? false : get().tagRequestArmed, contextMenu: null }),
     setDisplayedLayout: (layout) => {
      const normalizedLayout = normalizeLayout(layout);
      set({
        displayedLayout: normalizedLayout,
        followRecommendedLayout: normalizedLayout === get().layout,
      });
    },
    setLayoutChooserOpen: (open) => set({ layoutChooserOpen: open }),
    openStripContextMenu: (callsign, pos) => set({ contextMenu: { callsign, x: pos.x, y: pos.y } }),
    closeStripContextMenu: () => set({ contextMenu: null }),
    openValidationDialog,
    closeValidationDialog: () => set({ validationDialogCallsign: null }),
     move: (callsign, bay) => set((state) => {
          if (!sendGuardedStripEvent(callsign, { type: "move" }, {type: ActionType.FrontendMove, callsign, bay})) {
            return state;
          }

           return produce((state: WebSocketState) => {
             const stripIndex = state.strips.findIndex(strip => strip.callsign === callsign);
              if (stripIndex !== -1) {
                const currentBay = state.strips[stripIndex].bay;
                state.strips[stripIndex].bay = bay;
                state.strips[stripIndex].sequence = nextSequenceAtEndOfBay(state.strips, state.tacticalStrips, bay, callsign);
                if (currentBay === Bay.Stand && bay !== Bay.Stand) {
                  state.strips[stripIndex].start_req = false;
                }
                if (shouldResetRunwayClearanceOnMove(currentBay, bay)) {
                  state.strips[stripIndex].runway_cleared = false;
                  state.strips[stripIndex].runway_confirmed = false;
               }
             }
             return state;
           })(state)
       }
     ),
    generateSquawk: (callsign) => {
      return sendGuardedStripEvent(callsign, { type: "generate_squawk" }, {type: ActionType.FrontendGenerateSquawk, callsign});
    },
    updateStrip(callsign: string, update: UpdateStrip) {
      const fields = updateStripFieldsFromPayload(update);
      const normalizedEobt = update.eobt === undefined ? undefined : normalizeCdmTime(update.eobt);
      if (!sendGuardedStripEvent(callsign, { type: "update_strip", fields }, {
        type: ActionType.FrontendUpdateStripData,
        callsign,
        eobt: normalizedEobt,
        route: update.route,
        sid: update.sid,
        heading: update.heading,
        altitude: update.altitude,
        stand: update.stand,
        ob: update.ob,
        remarks: update.remarks,
        aircraft_type: update.aircraft_type,
      })) {
        return;
      }

      set((state) =>
        produce(state, (draft: WebSocketState) => {
          const stripIndex = draft.strips.findIndex(strip => strip.callsign === callsign);
          if (stripIndex !== -1) {
            if (update.sid !== undefined) {
              draft.strips[stripIndex].sid = update.sid;
            }
            if (update.eobt !== undefined) {
              draft.strips[stripIndex].eobt = normalizeCdmTime(update.eobt);
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
            if (update.remarks !== undefined) {
              draft.strips[stripIndex].remarks = update.remarks;
            }
            if (update.aircraft_type !== undefined) {
              draft.strips[stripIndex].aircraft_type = update.aircraft_type;
            }
            if (update.capabilities !== undefined) {
              draft.strips[stripIndex].capabilities = update.capabilities;
            }
          }
        })
      );
    },
    updateOrder: (callsign, insertAfter) => set((state) => {
      if (!sendGuardedStripEvent(callsign, { type: "update_order" }, {type: ActionType.FrontendUpdateOrder, callsign: callsign, insert_after: insertAfter})) {
        return state;
      }

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
      sendIfWritable({type: ActionType.FrontendSendMessage, text, recipients});
    },
    dismissMessage: (id) => {
      store.setState(
        produce((state: WebSocketState) => {
          state.messages = state.messages.filter(m => m.id !== id);
        })
      );
    },
    setReleasePoint: (callsign, releasePoint) => {
      if (!sendGuardedStripEvent(callsign, { type: "release_point" }, {type: ActionType.FrontendReleasePoint, callsign: callsign, release_point: releasePoint})) {
        return;
      }

      return produce((state: WebSocketState) => {
        const stripIndex = state.strips.findIndex(strip => strip.callsign === callsign)
        if (stripIndex !== -1) {
          state.strips[stripIndex].release_point = releasePoint
        }
      })
    },
    setStartReq: (callsign, startReq) => {
      if (!sendIfWritable({ type: ActionType.FrontendStartReq, callsign, start_req: startReq })) {
        return;
      }
      updateLocalStartReq(callsign, startReq);
    },
    issuePdcClearance: (callsign, remarks) => {
      if (!sendIfWritable({type: ActionType.FrontendIssuePdcClearanceRequest, callsign, remarks})) {
        return;
      }

      return produce((state: WebSocketState) => {
        const stripIndex = state.strips.findIndex(strip => strip.callsign === callsign)
        if (stripIndex !== -1) {
          state.strips[stripIndex].pdc_state = "CLEARED"
        }
      })
    },
    revertToVoice: (callsign) => {
      if (!sendIfWritable({type: ActionType.FrontendRevertToVoiceRequest, callsign})) {
        return;
      }

      return produce((state: WebSocketState) => {
        const stripIndex = state.strips.findIndex(strip => strip.callsign === callsign)
        if (stripIndex !== -1) {
          state.strips[stripIndex].pdc_state = "REVERT_TO_VOICE"
        }
      })
    },
    transferStrip: (callsign, toPosition) => {
      if (!sendGuardedStripEvent(callsign, { type: "coordination_transfer_request" }, {
        type: ActionType.FrontendCoordinationTransferRequest,
        callsign,
        to: toPosition,
      })) {
        return;
      }
      updateLocalStartReq(callsign, false);
    },
    assumeStrip: (callsign) => {
      sendGuardedStripEvent(callsign, { type: "coordination_assume_request" }, { type: ActionType.FrontendCoordinationAssumeRequest, callsign });
    },
    // forceAssumeStrip: takes ownership of an unowned strip, bypassing the next-owners check
    forceAssumeStrip: (callsign) => {
      sendIfWritable({ type: ActionType.FrontendCoordinationForceAssumeRequest, callsign });
    },
    // pickupStrip: assume if needed, then move to bay in one action (used when selecting from ARR/startup popups)
    pickupStrip: (callsign, bay) => {
      const strip = get().strips.find((candidate) => candidate.callsign === callsign);
      const myPosition = get().position;
      if (!strip || !myPosition || strip.owner !== myPosition) {
        get().forceAssumeStrip(callsign);
      }
      get().move(callsign, bay);
    },
    freeStrip: (callsign) => {
      sendGuardedStripEvent(callsign, { type: "coordination_free_request" }, { type: ActionType.FrontendCoordinationFreeRequest, callsign });
    },
     cancelTransfer: (callsign) => {
       sendIfWritable({ type: ActionType.FrontendCoordinationCancelTransferRequest, callsign });
     },
     requestTag: (callsign) => {
        if (!ensureWritable()) {
          return;
        }
        if (!guardValidationAction(callsign, { type: "coordination_tag_request" })) {
          return;
        }
        set({ tagRequestArmed: false });
        wsClient.send({ type: ActionType.FrontendCoordinationTagRequest, callsign });
       },
    acceptTagRequest: (callsign) => {
      sendGuardedStripEvent(callsign, { type: "coordination_accept_tag_request" }, { type: ActionType.FrontendCoordinationAcceptTagRequest, callsign });
    },
    cdmReady: (callsign) => {
      sendIfWritable({ type: ActionType.FrontendCdmReady, callsign });
    },
    clxUpdateTobt: (callsign) => {
      sendIfWritable({ type: ActionType.FrontendClxUpdateTobt, callsign });
    },
    clxOverrideValidation: (callsign, overrideKey) => {
      sendIfWritable({ type: ActionType.FrontendClxOverrideValidation, callsign, override_key: overrideKey });
    },
    assignRunway: (callsign, runway) => {
      if (!sendGuardedStripEvent(callsign, { type: "update_strip", fields: ["runway"] }, { type: ActionType.FrontendUpdateStripData, callsign, runway })) {
        return;
      }
      store.setState(
        produce((state: WebSocketState) => {
          const idx = state.strips.findIndex(s => s.callsign === callsign);
          if (idx !== -1) state.strips[idx].runway = runway;
        })
      );
    },
    toggleMarked: (callsign, marked) => {
      if (!ensureWritable()) {
        return;
      }
      set({ markArmed: false });
      wsClient.send({ type: ActionType.FrontendMarked, callsign, marked });
      store.setState(
        produce((state: WebSocketState) => {
          const idx = state.strips.findIndex(s => s.callsign === callsign);
          if (idx !== -1) state.strips[idx].marked = marked;
        })
      );
    },
     runwayClearance: (callsign) => {
       if (!sendGuardedStripEvent(callsign, { type: "runway_clearance" }, { type: ActionType.FrontendRunwayClearance, callsign })) {
         return;
       }
       store.setState(
         produce((state: WebSocketState) => {
           const idx = state.strips.findIndex(s => s.callsign === callsign);
           if (idx !== -1) {
             // Auto-confirm if no other strips on the same runway are already confirmed.
             const thisRunway = state.strips[idx].runway;
             const hasConfirmed = !!thisRunway && state.strips.some(s => s.callsign !== callsign && s.runway_confirmed && s.runway === thisRunway);
             const targetBay = runwayClearanceTargetBay(state.strips[idx].bay);
             state.strips[idx].runway_cleared = true;
             state.strips[idx].runway_confirmed = !hasConfirmed;
             if (targetBay !== null) {
               state.strips[idx].bay = targetBay;
               state.strips[idx].sequence = nextSequenceAtEndOfBay(state.strips, state.tacticalStrips, targetBay, callsign);
             }
           }
         })
       );
     },
     runwayConfirmation: (callsign) => {
      if (!sendGuardedStripEvent(callsign, { type: "runway_confirmation" }, { type: ActionType.FrontendRunwayConfirmation, callsign })) {
        return;
      }
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
      if (!sendIfWritable({ type: ActionType.FrontendAcknowledgeUnexpectedChange, callsign, field_name: fieldName })) {
        return;
      }
      store.setState(
        produce((state: WebSocketState) => {
          const idx = state.strips.findIndex(s => s.callsign === callsign);
          if (idx !== -1) {
            state.strips[idx].unexpected_change_fields = (state.strips[idx].unexpected_change_fields ?? []).filter(f => f !== fieldName);
            if (!(state.strips[idx].controller_modified_fields ?? []).includes(fieldName)) {
              state.strips[idx].controller_modified_fields = [
                ...(state.strips[idx].controller_modified_fields ?? []),
                fieldName,
              ];
            }
          }
        })
      );
    },
    missedApproach: (callsign) => {
      sendIfWritable({ type: ActionType.FrontendMissedApproach, callsign });
    },
    updateRunwayStatus: (pair, status) => {
      sendIfWritable({ type: ActionType.FrontendUpdateRunwayStatus, pair, status });
    },
    createManualFPL: (callsign, ades, sid, ssr, eobt, aircraftType, fl, route, stand, rwyDep) => {
      sendIfWritable({ type: ActionType.FrontendCreateManualFPL, callsign, ades, sid, ssr, eobt: normalizeCdmTime(eobt), aircraft_type: aircraftType, fl, route, stand, rwy_dep: rwyDep });
    },
    createVFRFPL: (callsign, aircraftType, personsOnBoard, ssr, fplType, language, remarks) => {
      sendIfWritable({ type: ActionType.FrontendCreateVFRFPL, callsign, aircraft_type: aircraftType, persons_on_board: personsOnBoard, ssr, fpl_type: fplType, language, remarks });
    },
    createTacticalStrip:(stripType, bay, label, aircraft) => {
      sendIfWritable({ type: ActionType.FrontendCreateTacticalStrip, strip_type: stripType, bay, label, aircraft });
    },
    deleteTacticalStrip: (id) => {
      sendIfWritable({ type: ActionType.FrontendDeleteTacticalStrip, id });
    },
    confirmTacticalStrip: (id) => {
      sendIfWritable({ type: ActionType.FrontendConfirmTacticalStrip, id });
    },
    startTacticalTimer: (id) => {
      sendIfWritable({ type: ActionType.FrontendStartTacticalTimer, id });
    },
    moveTacticalStrip: (id, insertAfter, bay) => set((state) => {
      if (!sendIfWritable({ type: ActionType.FrontendMoveTacticalStrip, id, insert_after: insertAfter, bay })) {
        return state;
      }

      return produce((draft: WebSocketState) => {
        const idx = draft.tacticalStrips.findIndex(t => t.id === id);
        if (idx === -1) return;

        const targetBay = bay ?? draft.tacticalStrips[idx].bay;

        // All sequences in the bay except the strip being moved, sorted ascending
        const baySeqs = [
          ...draft.strips.filter(s => s.bay === targetBay).map(s => s.sequence),
          ...draft.tacticalStrips.filter(t => t.bay === targetBay && t.id !== id).map(t => t.sequence),
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

        draft.tacticalStrips[idx].bay = targetBay;
        draft.tacticalStrips[idx].sequence = nextSeq === null
          ? prevSeq + 100
          : Math.floor((prevSeq + nextSeq) / 2);
      })(state);
    }),

    acknowledgeValidationStatus: (callsign, activationKey) => {
      sendIfWritable({ type: ActionType.FrontendAcknowledgeValidationStatus, callsign, activation_key: activationKey });
    },
  }});

  // Private methods to handle WebSocket events
  const handleInitialEvent = (data: FrontendInitialEvent) => {
    wsClient.setReadOnly(data.read_only ?? false);
    store.setState(
      produce((state: WebSocketState) => {
        state.controllers = data.controllers.map(c => ({ ...c, owned_sectors: c.owned_sectors ?? [] }));
        state.strips = data.strips.map(strip => ({
          ...strip,
          eobt: normalizeCdmTime(strip.eobt),
          tobt: normalizeCdmTime(strip.tobt),
          tsat: normalizeCdmTime(strip.tsat),
          ctot: normalizeCdmTime(strip.ctot),
        }));
        state.tacticalStrips = data.tactical_strips ?? [];
        state.position = data.me.position;
        state.identifier = data.me.identifier;
        state.airport = data.airport;
        state.callsign = data.callsign;
        state.readOnly = data.read_only ?? false;
        state.positionAvailable = data.position_available ?? true;
        const normalizedLayout = normalizeLayout(data.layout);
        state.layout = normalizedLayout;
        if (KNOWN_LAYOUTS.has(normalizedLayout)) {
          state.displayedLayout = normalizedLayout;
          state.followRecommendedLayout = true;
        } else {
          state.displayedLayout = "";
          state.followRecommendedLayout = true;
        }
        state.runwaySetup = data.runway_setup;
        state.initialCflByRunway = data.initial_cfl_by_runway ?? {};
        state.transitionAltitude = data.transition_altitude ?? 0;
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

    if ((data.read_only ?? false) && !(data.position_available ?? true) && data.me.position) {
      toast.error("Invalid observer frequency", {
        description: `Primary frequency ${data.me.position} does not match any online controller. Select a primary frequency that matches an active controller.`,
      });
    }
  };

  const handleStripUpdateEvent = (data: FrontendStripUpdateEvent) => {
    store.setState(
      produce((state: WebSocketState) => {
        const stripIndex = state.strips.findIndex(strip => strip.callsign === data.callsign);

        if (stripIndex !== -1) {
          // Update existing strip
          state.strips[stripIndex] = {
            ...data,
            eobt: normalizeCdmTime(data.eobt),
            tobt: normalizeCdmTime(data.tobt),
            tsat: normalizeCdmTime(data.tsat),
            ctot: normalizeCdmTime(data.ctot),
          };
        } else {
          // Add new strip
          state.strips.push({
            ...data,
            eobt: normalizeCdmTime(data.eobt),
            tobt: normalizeCdmTime(data.tobt),
            tsat: normalizeCdmTime(data.tsat),
            ctot: normalizeCdmTime(data.ctot),
          });
        }
      })
    );
  };

  const handleControllerOnlineEvent = (
    data: FrontendControllerOnlineEvent | FrontendControllerUpdateEvent
  ) => {
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
            owned_sectors: data.owned_sectors ?? [],
          });
        } else {
          state.controllers[controllerIndex] = {
            ...state.controllers[controllerIndex],
            callsign: data.callsign,
            position: data.position,
            identifier: data.identifier,
            section: data.section,
            owned_sectors: data.owned_sectors ?? [],
          };
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

  const handleDisconnectEvent = (data: FrontendDisconnectEvent) => {
    const readOnly = data.read_only ?? false;
    wsClient.setReadOnly(readOnly);
    store.setState({
      ...initialState,
      readOnly,
    })
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
        const normalizedLayout = normalizeLayout(data.layout);
        state.layout = normalizedLayout;
        if (KNOWN_LAYOUTS.has(normalizedLayout)) {
          if (state.followRecommendedLayout) {
            state.displayedLayout = normalizedLayout;
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
          state.strips[stripIndex].tobt = normalizeCdmTime(data.tobt)
          state.strips[stripIndex].eobt = normalizeCdmTime(data.eobt)
          state.strips[stripIndex].tsat = normalizeCdmTime(data.tsat)
          state.strips[stripIndex].ctot = normalizeCdmTime(data.ctot)
          if (data.req_tobt !== undefined) state.strips[stripIndex].req_tobt = normalizeCdmTime(data.req_tobt)
          if (data.ttot !== undefined) state.strips[stripIndex].ttot = normalizeCdmTime(data.ttot)
          if (data.aobt !== undefined) state.strips[stripIndex].aobt = normalizeCdmTime(data.aobt)
          if (data.asat !== undefined) state.strips[stripIndex].asat = normalizeCdmTime(data.asat)
          if (data.asrt !== undefined) state.strips[stripIndex].asrt = normalizeCdmTime(data.asrt)
          if (data.tsac !== undefined) state.strips[stripIndex].tsac = normalizeCdmTime(data.tsac)
          if (data.status !== undefined) state.strips[stripIndex].status = data.status
          if (data.ecfmp_id !== undefined) state.strips[stripIndex].ecfmp_id = data.ecfmp_id
          if (data.ctot_source !== undefined) state.strips[stripIndex].ctot_source = data.ctot_source
          if (data.phase !== undefined) state.strips[stripIndex].phase = data.phase
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
            state.strips[stripIndex].pdc_request_remarks = data.pdc_request_remarks ?? "";
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

  const handleGoAroundEvent = (_data: FrontendGoAroundEvent) => {
    if (!isAudioMuted()) {
      new Audio(missedApproachSound).play().catch(() => {});
    }
  };

  // Register event handlers
  wsClient.on(EventType.FrontendInitial, handleInitialEvent);
  wsClient.on(EventType.FrontendGoAround, handleGoAroundEvent);
  wsClient.on(EventType.FrontendStripUpdate, handleStripUpdateEvent);
  wsClient.on(EventType.FrontendControllerOnline, handleControllerOnlineEvent);
  wsClient.on(EventType.FrontendControllerUpdate, handleControllerOnlineEvent);
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

  const handleActionRejectedEvent = () => {
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
