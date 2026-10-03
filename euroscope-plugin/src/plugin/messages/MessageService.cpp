#include "MessageService.h"
#include "flightplan/TopSkyHold.h"
#include "PrivateMessageSender.h"
#include "websocket/ProtoCodec.h"
#include "websocket/generated/proto/euroscope.pb.h"

#include "Logger.hpp"

namespace FlightStrips::messages {
    namespace {
        bool IsAirborneCapablePosition(const std::string &callsign) {
            size_t segmentStart = 0;
            while (segmentStart <= callsign.length()) {
                const auto separatorIndex = callsign.find('_', segmentStart);
                const auto segment = callsign.substr(
                    segmentStart,
                    separatorIndex == std::string::npos ? std::string::npos : separatorIndex - segmentStart
                );
                if (_stricmp(segment.c_str(), "APP") == 0 ||
                    _stricmp(segment.c_str(), "DEP") == 0 ||
                    _stricmp(segment.c_str(), "CTR") == 0) {
                    return true;
                }
                if (separatorIndex == std::string::npos) return false;
                segmentStart = separatorIndex + 1;
            }

            return false;
        }

        bool ShouldMirrorClearedFlagToEuroScope(const flightplan::FlightPlan* plan, const bool cleared) {
            if (!cleared || plan == nullptr) {
                return true;
            }

            return !plan->KeepsEuroScopeStripUncleared();
        }

        std::string JoinRunways(const std::vector<std::string> &runways) {
            if (runways.empty()) {
                return "none";
            }

            std::string joined;
            for (size_t index = 0; index < runways.size(); ++index) {
                if (index > 0) {
                    joined += ", ";
                }
                joined += runways[index];
            }
            return joined;
        }
    }

    void MessageService::OnMessages(const std::vector<std::string> &messages) {
        for (const auto &message: messages) {
            HandleMessage(message);
        }
    }

    void MessageService::HandleMessage(const std::string &message) const {
        try {
            websocket::protobuf::wire::Envelope envelope;
            if (!websocket::protobuf::ParseEnvelope(message, envelope)) {
                Logger::Warning("Invalid protobuf message ({} bytes)", message.size());
                return;
            }
			if (envelope.event_case() == websocket::protobuf::wire::Envelope::kResultRecorded) {
				if (envelope.command_id() == envelope.result_recorded().command_id())
					m_webSocketService->AcknowledgeCommandResult(envelope.result_recorded().command_id());
				return;
			}
			if (envelope.event_case() == websocket::protobuf::wire::Envelope::kSessionInfo)
				m_webSocketService->SetSessionTerms(envelope.session_id(),
					envelope.session_info().owner_epoch(), envelope.session_info().master_epoch());
			if (!envelope.command_id().empty()) {
				if (!m_webSocketService->BeginCommand(envelope.command_id())) return;
				if (envelope.event_case() == websocket::protobuf::wire::Envelope::kGenerateSquawk) {
					using Result = websocket::protobuf::wire::CommandResultEvent;
					const auto& callsign = envelope.generate_squawk().callsign();
					const auto fp = m_plugin->FlightPlanSelect(callsign.c_str());
					if (!callsign.empty() && fp.IsValid() && !m_plugin->GetConnectionState().observer) {
						m_plugin->AddSquawkCommand(callsign, envelope.command_id(), envelope.session_id(),
							envelope.owner_epoch(), envelope.master_epoch());
					} else {
						m_webSocketService->RecordCommandResult(envelope.command_id(), Result::FAILED,
							callsign.empty() ? Result::INVALID_ARGUMENT : Result::TARGET_NOT_FOUND,
							"operational flight plan unavailable", envelope.session_id(), envelope.owner_epoch(), envelope.master_epoch());
					}
					return;
				}
				CommandOutcome outcome{};
				try {
					outcome = ExecuteCommand(envelope);
				} catch (const std::exception& e) {
					Logger::Error("EuroScope command {} failed: {}", envelope.command_id(), e.what());
					outcome = {websocket::protobuf::wire::CommandResultEvent::FAILED,
					           websocket::protobuf::wire::CommandResultEvent::INTERNAL_ERROR, "local exception"};
				} catch (...) {
					outcome = {websocket::protobuf::wire::CommandResultEvent::FAILED,
					           websocket::protobuf::wire::CommandResultEvent::INTERNAL_ERROR, "local exception"};
				}
				m_webSocketService->RecordCommandResult(envelope.command_id(), outcome.status, outcome.reason, outcome.detail,
					envelope.session_id(), envelope.owner_epoch(), envelope.master_epoch());
				return;
			}

#define HANDLE_PROTO(caseName, accessor, domainType, handler, typeName) \
            case websocket::protobuf::wire::Envelope::caseName: { \
                if (!m_webSocketService->ShouldProcessServerMessageType(typeName)) return; \
                domainType event; \
                websocket::protobuf::Decode(envelope.accessor(), event); \
                handler(event); \
                break; \
            }

            Logger::Info("Received protobuf event type {}", static_cast<int>(envelope.event_case()));
            switch (envelope.event_case()) {
            HANDLE_PROTO(kSessionInfo, session_info, SessionInfoEvent, HandleSessionInfoEvent, EVENT_SESSION_INFO_NAME)
            HANDLE_PROTO(kRunwayMismatchAlert, runway_mismatch_alert, RunwayMismatchAlertEvent, HandleRunwayMismatchAlertEvent, EVENT_RUNWAY_MISMATCH_ALERT_NAME)
            HANDLE_PROTO(kCdmUpdate, cdm_update, CdmUpdateEvent, HandleCdmUpdateEvent, EVENT_CDM_UPDATE_NAME)
            HANDLE_PROTO(kCdmUpdateBatch, cdm_update_batch, CdmUpdateBatchEvent, HandleCdmUpdateBatchEvent, EVENT_CDM_UPDATE_BATCH_NAME)
            HANDLE_PROTO(kAssignedSquawk, assigned_squawk, AssignedSquawkEvent, HandleAssignedSquawkEvent, EVENT_ASSIGNED_SQUAWK_NAME)
            HANDLE_PROTO(kRequestedAltitude, requested_altitude, RequestedAltitudeEvent, HandleRequestedAltitudeEvent, EVENT_REQUESTED_ALTITUDE_NAME)
            HANDLE_PROTO(kClearedAltitude, cleared_altitude, ClearedAltitudeEvent, HandleClearedAltitudeEvent, EVENT_CLEARED_ALTITUDE_NAME)
            HANDLE_PROTO(kCommunicationType, communication_type, CommunicationTypeEvent, HandleCommunicationTypeEvent, EVENT_COMMUNICATION_TYPE_NAME)
            HANDLE_PROTO(kGroundState, ground_state, GroundStateEvent, HandleGroundStateEvent, EVENT_GROUND_STATE_NAME)
            HANDLE_PROTO(kClearedFlag, cleared_flag, ClearedFlagEvent, HandleClearedFlagEvent, EVENT_CLEARED_FLAG_NAME)
            HANDLE_PROTO(kHeading, heading, HeadingEvent, HandleHeadingEvent, EVENT_HEADING_NAME)
            HANDLE_PROTO(kStand, stand, StandEvent, HandleStandEvent, EVENT_STAND_NAME)
            HANDLE_PROTO(kEobt, eobt, EobtEvent, HandleEobtEvent, EVENT_EOBT_NAME)
            HANDLE_PROTO(kGenerateSquawk, generate_squawk, GenerateSquawkEvent, HandleGenerateSquawkEvent, EVENT_GENERATE_SQUAWK_NAME)
            HANDLE_PROTO(kRoute, route, RouteEvent, HandleRouteEvent, EVENT_ROUTE_NAME)
            HANDLE_PROTO(kRemarks, remarks, RemarksEvent, HandleRemarksEvent, EVENT_REMARKS_NAME)
            HANDLE_PROTO(kAircraftInfo, aircraft_info, AircraftInfoEvent, HandleAircraftInfoEvent, EVENT_AIRCRAFT_INFO_NAME)
            HANDLE_PROTO(kAircraftInfoRemarks, aircraft_info_remarks, AircraftInfoRemarksEvent, HandleAircraftInfoRemarksEvent, EVENT_AIRCRAFT_INFO_REMARKS_NAME)
            HANDLE_PROTO(kSid, sid, SidEvent, HandleSidEvent, EVENT_SID_NAME)
            HANDLE_PROTO(kAircraftRunway, aircraft_runway, AircraftRunwayEvent, HandleAircraftRunwayEvent, EVENT_AIRCRAFT_RUNWAY_NAME)
            HANDLE_PROTO(kCoordinationHandover, coordination_handover, CoordinationHandoverEvent, HandleCoordinationHandoverEvent, EVENT_COORDINATION_HANDOVER_NAME)
            HANDLE_PROTO(kAssumeOnly, assume_only, AssumeOnlyEvent, HandleEsAssumeOnlyEvent, EVENT_ASSUME_ONLY_NAME)
            HANDLE_PROTO(kAssumeAndDrop, assume_and_drop, AssumeAndDropEvent, HandleEsAssumeAndDropEvent, EVENT_ASSUME_AND_DROP_NAME)
            HANDLE_PROTO(kDropTracking, drop_tracking, DropTrackingEvent, HandleEsDropTrackingEvent, EVENT_DROP_TRACKING_NAME)
            HANDLE_PROTO(kBackendSync, backend_sync, BackendSyncEvent, HandleBackendSyncEvent, EVENT_BACKEND_SYNC_NAME)
            HANDLE_PROTO(kCreateFpl, create_fpl, CreateFPLEvent, HandleCreateFPLEvent, EVENT_CREATE_FPL_NAME)
            HANDLE_PROTO(kPdcStateChange, pdc_state_change, PdcStateChangeEvent, HandlePdcStateChangeEvent, EVENT_PDC_STATE_CHANGE_NAME)
            HANDLE_PROTO(kSendPrivateMessage, send_private_message, SendPrivateMessageEvent, HandleSendPrivateMessageEvent, EVENT_SEND_PRIVATE_MESSAGE_NAME)
            HANDLE_PROTO(kHold, hold, HoldEvent, HandleHoldEvent, EVENT_HOLD_NAME)
            HANDLE_PROTO(kTrackingControllerChanged, tracking_controller_changed, TrackingControllerChangedEvent, HandleTrackingControllerChangedEvent, EVENT_TRACKING_CONTROLLER_CHANGED_NAME)
            case websocket::protobuf::wire::Envelope::kAmanGainLoss:
                // Consumed by AMANGainLossStore, which is registered separately.
                break;
            default:
                Logger::Warning("Unsupported server protobuf event type {}", static_cast<int>(envelope.event_case()));
                break;
            }
#undef HANDLE_PROTO

        } catch (const std::exception &e) {
            Logger::Error("Exception handling message: {}", e.what());
        } catch (...) {
            Logger::Error("Unknown exception handling message");
        }
    }

    MessageService::CommandOutcome MessageService::ExecuteCommand(
        const websocket::protobuf::wire::Envelope& envelope) const {
        using namespace websocket::protobuf;
        using Result = wire::CommandResultEvent;
        const auto ok = []() -> CommandOutcome { return {Result::EXECUTED, Result::OK, {}}; };
        const auto fail = [](Result::Reason reason, const char* detail) -> CommandOutcome {
            return {Result::FAILED, reason, detail};
        };
        const auto withPlan = [this, &fail](const std::string& callsign, auto operation) -> CommandOutcome {
            if (callsign.empty()) return fail(Result::INVALID_ARGUMENT, "missing callsign");
            auto fp = m_plugin->FlightPlanSelect(callsign.c_str());
            if (!fp.IsValid()) return fail(Result::TARGET_NOT_FOUND, "flight plan not found");
            return operation(fp);
        };
        const auto setter = [&ok, &fail](bool success) -> CommandOutcome {
            return success ? ok() : fail(Result::EUROSCOPE_API_REJECTED, "EuroScope setter rejected change");
        };
        const auto amend = [&ok, &fail](auto fpData, bool set) -> CommandOutcome {
            if (!set) return fail(Result::EUROSCOPE_API_REJECTED, "flight plan setter rejected change");
            if (!fpData.AmendFlightPlan())
                return fail(Result::PARTIAL_EXECUTION, "setter applied but flight plan amend failed");
            return ok();
        };

        switch (envelope.event_case()) {
        case wire::Envelope::kAssignedSquawk:
            return withPlan(envelope.assigned_squawk().callsign(), [&](auto fp) {
                return setter(fp.GetControllerAssignedData().SetSquawk(envelope.assigned_squawk().squawk().c_str()));
            });
        case wire::Envelope::kRequestedAltitude:
            return withPlan(envelope.requested_altitude().callsign(), [&](auto fp) {
                return setter(fp.GetControllerAssignedData().SetFinalAltitude(envelope.requested_altitude().altitude()));
            });
        case wire::Envelope::kClearedAltitude:
            return withPlan(envelope.cleared_altitude().callsign(), [&](auto fp) {
                return setter(fp.GetControllerAssignedData().SetClearedAltitude(envelope.cleared_altitude().altitude()));
            });
        case wire::Envelope::kHeading:
            return withPlan(envelope.heading().callsign(), [&](auto fp) {
                return setter(fp.GetControllerAssignedData().SetAssignedHeading(envelope.heading().heading()));
            });
        case wire::Envelope::kCommunicationType:
            return withPlan(envelope.communication_type().callsign(), [&](auto fp) {
                const auto& value = envelope.communication_type().communication_type();
                if (value.size() != 1) return fail(Result::INVALID_ARGUMENT, "invalid communication type");
                return setter(fp.GetControllerAssignedData().SetCommunicationType(value[0]));
            });
        case wire::Envelope::kEobt:
            return withPlan(envelope.eobt().callsign(), [&](auto fp) {
                auto data = fp.GetFlightPlanData();
                return amend(data, data.SetEstimatedDepartureTime(envelope.eobt().eobt().c_str()));
            });
        case wire::Envelope::kRoute:
            return withPlan(envelope.route().callsign(), [&](auto fp) {
                auto data = fp.GetFlightPlanData();
                return amend(data, data.SetRoute(envelope.route().route().c_str()));
            });
        case wire::Envelope::kRemarks:
            return withPlan(envelope.remarks().callsign(), [&](auto fp) {
                auto data = fp.GetFlightPlanData();
                return amend(data, data.SetRemarks(envelope.remarks().remarks().c_str()));
            });
        case wire::Envelope::kAircraftInfo:
            return withPlan(envelope.aircraft_info().callsign(), [&](auto fp) {
                auto data = fp.GetFlightPlanData();
                return amend(data, data.SetAircraftInfo(envelope.aircraft_info().aircraft_type().c_str()));
            });
        case wire::Envelope::kAircraftInfoRemarks:
            return withPlan(envelope.aircraft_info_remarks().callsign(), [&](auto fp) {
                auto data = fp.GetFlightPlanData();
                if (!data.SetAircraftInfo(envelope.aircraft_info_remarks().aircraft_type().c_str()))
                    return fail(Result::EUROSCOPE_API_REJECTED, "aircraft info setter rejected change");
                if (!data.SetRemarks(envelope.aircraft_info_remarks().remarks().c_str()))
                    return fail(Result::PARTIAL_EXECUTION, "aircraft info applied but remarks setter failed");
                return amend(data, true);
            });
        case wire::Envelope::kSid:
        case wire::Envelope::kAircraftRunway: {
            const bool sid = envelope.event_case() == wire::Envelope::kSid;
            const auto& callsign = sid ? envelope.sid().callsign() : envelope.aircraft_runway().callsign();
            return withPlan(callsign, [&](auto fp) {
                auto data = fp.GetFlightPlanData();
                auto route = std::string(data.GetRoute());
                const auto airport = m_plugin->GetConnectionState().relevant_airport;
                if (sid) m_routeService->SetSid(route, envelope.sid().sid(), airport);
                else {
                    if (_stricmp(data.GetOrigin(), airport.c_str()) != 0)
                        return fail(Result::INVALID_ARGUMENT, "runway command is not a departure");
                    m_routeService->SetDepartureRunway(route, envelope.aircraft_runway().runway(), airport);
                }
                if (route.empty()) return fail(Result::INVALID_ARGUMENT, "empty route after change");
                return amend(data, data.SetRoute(route.c_str()));
            });
        }
        case wire::Envelope::kGroundState:
            return setter(m_plugin->UpdateViaScratchPad(envelope.ground_state().callsign().c_str(),
                                                         envelope.ground_state().ground_state().c_str()));
        case wire::Envelope::kHold:
            return withPlan(envelope.hold().callsign(), [&](auto fp) {
                const auto& hold = envelope.hold();
                if (hold.hold_eat().empty()) {
                    // Withdrawal removes AMAN's replay authority. As in the
                    // existing Hold handler, it never cancels a controller hold
                    // or invents an unsupported empty TopSky EAT pulse.
                    m_flightPlanService->CacheBackendHoldEatReplay(hold.callsign(), hold.hold(), hold.hold_type(), "");
                    return ok();
                }
                if (!fp.GetTrackingControllerIsMe())
                    return fail(Result::INVALID_ARGUMENT, "holding EAT requires the tracking controller");
                const auto annotation = fp.GetControllerAssignedData().GetFlightStripAnnotation(flightplan::TOPSKY_HOLD_ANNOTATION);
                const auto command = flightplan::BuildTopSkyHoldEatCommand(
                    flightplan::ParseTopSkyHoldAnnotation(annotation == nullptr ? "" : annotation),
                    hold.hold(), hold.hold_type(), hold.hold_eat());
                if (command.empty()) return fail(Result::INVALID_ARGUMENT, "live hold differs from AMAN intent");
                if (!m_plugin->UpdateViaScratchPad(hold.callsign().c_str(), command.c_str()))
                    return fail(Result::EUROSCOPE_API_REJECTED, "TopSky holding EAT rejected");
                m_flightPlanService->CacheBackendHoldEatReplay(hold.callsign(), hold.hold(), hold.hold_type(), hold.hold_eat());
                return ok();
            });
        case wire::Envelope::kStand:
            return setter(m_plugin->SetArrivalStand(envelope.stand().callsign(), envelope.stand().stand()));
        case wire::Envelope::kClearedFlag:
			if (!ShouldMirrorClearedFlagToEuroScope(
				m_flightPlanService->GetFlightPlan(envelope.cleared_flag().callsign()),
				envelope.cleared_flag().cleared())) return ok();
            return setter(m_plugin->SetClearenceFlag(envelope.cleared_flag().callsign(),
                                                      envelope.cleared_flag().cleared()));
        case wire::Envelope::kSendPrivateMessage:
            if (envelope.send_private_message().callsign().empty() || envelope.send_private_message().message().empty())
                return fail(Result::INVALID_ARGUMENT, "missing recipient or message");
            return PrivateMessageSender::SendPrivateMessage(envelope.send_private_message().callsign(),
                envelope.send_private_message().message()) ? ok() : fail(Result::UI_UNAVAILABLE, "message input unavailable");
        case wire::Envelope::kCoordinationHandover:
            return withPlan(envelope.coordination_handover().callsign(), [&](auto fp) {
                const auto& target = envelope.coordination_handover().target_callsign();
                auto controller = m_plugin->ControllerSelect(target.c_str());
                if (!controller.IsValid() || !controller.IsController())
                    return fail(Result::TARGET_NOT_FOUND, "target controller not found");
                if (!fp.GetTrackingControllerIsMe() && !fp.StartTracking())
                    return fail(Result::EUROSCOPE_API_REJECTED, "start tracking rejected");
                if (!fp.InitiateHandoff(target.c_str()))
                    return fail(Result::PARTIAL_EXECUTION, "tracking started but handoff rejected");
                return ok();
            });
        case wire::Envelope::kAssumeOnly:
        case wire::Envelope::kAssumeAndDrop: {
            const bool drop = envelope.event_case() == wire::Envelope::kAssumeAndDrop;
            const auto& callsign = drop ? envelope.assume_and_drop().callsign() : envelope.assume_only().callsign();
            return withPlan(callsign, [&](auto fp) {
                if (fp.GetState() != EuroScopePlugIn::FLIGHT_PLAN_STATE_TRANSFER_TO_ME_INITIATED)
                    return fail(Result::INVALID_ARGUMENT, "flight plan is not offered for handoff");
                fp.AcceptHandoff();
                if (drop && !fp.EndTracking()) return fail(Result::PARTIAL_EXECUTION, "handoff accepted but drop rejected");
                return ok();
            });
        }
        case wire::Envelope::kDropTracking:
            return withPlan(envelope.drop_tracking().callsign(), [&](auto fp) {
                return setter(fp.EndTracking());
            });
        case wire::Envelope::kCdmUpdate: {
            CdmUpdateEvent event;
            Decode(envelope.cdm_update(), event);
            HandleCdmUpdateEvent(event);
            return ok();
        }
        case wire::Envelope::kPdcStateChange: {
            PdcStateChangeEvent event;
            Decode(envelope.pdc_state_change(), event);
            HandlePdcStateChangeEvent(event);
            return ok();
        }
		case wire::Envelope::kCreateFpl: {
			CreateFPLEvent event;
			Decode(envelope.create_fpl(), event);
			return HandleCreateFPLEvent(event);
		}
        default:
            return fail(Result::INVALID_ARGUMENT, "unsupported command event");
        }
    }

    void MessageService::HandleCdmUpdateEvent(const CdmUpdateEvent &event) const {
        m_flightPlanService->ApplyCdmUpdate(event);
    }

    void MessageService::HandleCdmUpdateBatchEvent(const CdmUpdateBatchEvent &event) const {
        for (const auto& update : event.updates) {
            m_flightPlanService->ApplyCdmUpdate(update);
        }
    }

    void MessageService::HandleSessionInfoEvent(const SessionInfoEvent &event) const {
        auto state = websocket::STATE_SLAVE;
        if (event.role == "master") {
            state = websocket::STATE_MASTER;
        } else if (event.role == "observer") {
            state = websocket::STATE_OBSERVER;
        }
        m_webSocketService->SetSessionState(state);

        const auto replayHolds = [&] {
            // A tracking client may reconcile an active annotation after reconnect.
            // Missing local annotation state is never inferred as a cancellation;
            // an explicitly cached XHOLD remains authoritative and is replayed.
            for (auto it = m_plugin->FlightPlanSelectFirst(); it.IsValid(); it = m_plugin->FlightPlanSelectNext(it)) {
                if (m_plugin->IsRelevant(it)) m_flightPlanService->ReplayTrackedHold(it);
            }

            // The master can repair commands observed during a backend outage even
            // when the tracking controller is not running FlightStrips.
            if (state == websocket::STATE_MASTER) {
                m_flightPlanService->ReplayPendingHoldCommands();
            }

        };

        Logger::Debug("Is master: {}", state == websocket::STATE_MASTER);

        if (state != websocket::STATE_MASTER) {
            replayHolds();
            // Send our runway config so the backend can detect conflicts with the master
            const auto airport = m_plugin->GetConnectionState().relevant_airport;
            if (!airport.empty()) {
                const auto runwayEvent = RunwayEvent(m_runwayService->GetActiveRunways(airport.c_str()));
                m_webSocketService->SendEvent(runwayEvent);
            }
            return;
        }

        // send sync event
        std::vector<Controller> controllers;
        std::vector<Strip> strips;

        for (auto it = m_plugin->ControllerSelectFirst(); it.IsValid(); it = m_plugin->ControllerSelectNext(it)) {
            if (!it.IsController()) continue;
            const auto primaryFrequency = std::format("{:.3f}", it.GetPrimaryFrequency());
            controllers.emplace_back(primaryFrequency, std::string(it.GetCallsign()));
        }

        const auto relevantAirport = m_plugin->GetConnectionState().relevant_airport.c_str();
        for (auto it = m_plugin->FlightPlanSelectFirst(); it.IsValid(); it = m_plugin->FlightPlanSelectNext(it)) {
            if (!m_plugin->IsRelevant(it)) continue;;
            if (it.GetSimulated()) continue;;
            const auto flightPlanData = it.GetFlightPlanData();

            const auto callsign = std::string(it.GetCallsign());
            const auto remarks = std::string(flightPlanData.GetRemarks());
            const auto radarTarget = m_plugin->RadarTargetSelect(callsign.c_str());
            const auto radarPosition = radarTarget.GetPosition();
            const auto hasRadarPosition = radarPosition.IsValid();
            double latitude = 0.0;
            double longitude = 0.0;
            int altitude = 0;
            std::string squawk;
            if (hasRadarPosition) {
                const auto position = radarPosition.GetPosition();
                latitude = position.m_Latitude;
                longitude = position.m_Longitude;
                altitude = radarPosition.GetPressureAltitude();
                squawk = std::string(radarPosition.GetSquawk());
            }
            const auto info = m_flightPlanService->GetFlightPlan(callsign);
            const auto isArrival = strcmp(it.GetFlightPlanData().GetDestination(), relevantAirport) == 0;
            const auto runway = std::string(isArrival
                                                ? it.GetFlightPlanData().GetArrivalRwy()
                                                : it.GetFlightPlanData().GetDepartureRwy());
            const auto controllerAssignedData = it.GetControllerAssignedData();
            const auto holdAnnotation = controllerAssignedData.GetFlightStripAnnotation(flightplan::TOPSKY_HOLD_ANNOTATION);
            const auto hold = flightplan::ParseTopSkyHoldAnnotation(holdAnnotation == nullptr ? "" : holdAnnotation);
            std::string holdEat;
            if (info != nullptr && info->hold == hold.point && info->hold_type == hold.TypeName()) {
                holdEat = info->hold_eat;
            }
            std::string stand;
            if (info != nullptr) {
                stand = info->stand;
            }

            if (stand.empty() && hasRadarPosition && altitude < 1000) {
                if (const auto standPtr = m_standService->GetStandFromFlightPlan(it, radarTarget); standPtr != nullptr) {
                    stand = standPtr->GetName();
                    m_flightPlanService->SetStand(callsign, stand);
                }
            }

            strips.push_back({
                callsign,
                std::string(flightPlanData.GetOrigin()),
                std::string(flightPlanData.GetDestination()),
                std::string(flightPlanData.GetAlternate()),
                std::string(flightPlanData.GetRoute()),
                remarks,
                runway,
                squawk,
                std::string(controllerAssignedData.GetSquawk()),
                std::string(flightPlanData.GetSidName()),
                isArrival ? std::string(flightPlanData.GetStarName()) : "",
                it.GetClearenceFlag(),
                std::string(it.GetGroundState()),
                controllerAssignedData.GetClearedAltitude(),
                flightPlanData.GetFinalAltitude(),
                controllerAssignedData.GetAssignedHeading(),
                std::string(flightPlanData.GetAircraftInfo()),
                {flightPlanData.GetAircraftWtc()},
                m_flightPlanService->ResolveSpokenCallsign(callsign, remarks),
                Position{
                    latitude, longitude, altitude
                },
                stand,
                {flightPlanData.GetCommunicationType()},
                flightPlanData.GetCapibilities() == 0 ? "?" : std::string {flightPlanData.GetCapibilities()},
                isArrival ? "" : std::string(flightPlanData.GetEstimatedDepartureTime()),
                isArrival ? flightplan::FlightPlanService::GetEstimatedLandingTime(it) : "",
                std::string(it.GetTrackingControllerCallsign()),
                {flightPlanData.GetEngineType()},
                true,
                it.GetTrackingControllerIsMe() ? hold.point : "",
                it.GetTrackingControllerIsMe() ? hold.TypeName() : "",
                it.GetTrackingControllerIsMe() ? holdEat : "",
                it.GetTrackingControllerIsMe()
            });
        }

        // Include no-FP (VFR) aircraft in range. FlightPlanSelectFirst only yields aircraft with a
        // correlated flight plan, so no-FP radar targets must be iterated separately. Without this,
        // the backend never receives a strip_update for them and every subsequent position event
        // is silently dropped with "strip does not exist".
        for (auto rt = m_plugin->RadarTargetSelectFirst(); rt.IsValid(); rt = m_plugin->RadarTargetSelectNext(rt)) {
            const auto callsign = std::string(rt.GetCallsign());
            // Skip any aircraft with a live FP — those are handled by the FlightPlanSelectFirst loop above.
            const auto rtFp = m_plugin->FlightPlanSelect(callsign.c_str());
            if (rtFp.IsValid()) continue;

            const auto position = rt.GetPosition();
            if (!position.IsValid()) continue;

            const auto pos = position.GetPosition();
            const auto info = m_flightPlanService->GetFlightPlan(callsign);
            std::string stand;
            if (info != nullptr && !info->stand.empty()) {
                stand = info->stand;
            } else if (position.GetPressureAltitude() < 1000) {
                if (const auto s = m_standService->GetStand(pos); s != nullptr) {
                    stand = s->GetName();
                    m_flightPlanService->SetStand(callsign, stand);
                }
            }

            strips.push_back({
                callsign,
                "", "",  // origin, destination — unknown for VFR
                "", "", "", "",  // alternate, route, remarks, runway
                std::string(position.GetSquawk()), "", "", "",  // squawk, assigned_squawk, sid, star
                false, "",   // cleared, ground_state
                0, 0, 0,    // cleared_altitude, requested_altitude, heading
                "", "", "", // aircraft_type, aircraft_category, spoken_callsign
                Position{pos.m_Latitude, pos.m_Longitude, position.GetPressureAltitude()},
                stand,
                "", "", "",  // communication_type, capabilities, eobt
                "",          // eldt
                "",          // tracking_controller
                "",          // engine_type
                false        // has_fp — no flight plan received
            });
        }

        const auto syncEvent = SyncEvent(strips, controllers, m_runwayService->GetActiveRunways(relevantAirport), [&] {
            std::vector<SidEntry> sidEntries;
            for (const auto& sid : m_plugin->GetSids(relevantAirport)) {
                sidEntries.push_back(SidEntry{sid.name, sid.runway});
            }
            return sidEntries;
        }());
        m_webSocketService->SendEvent(syncEvent);
        replayHolds();
    }

    void MessageService::HandleAssignedSquawkEvent(const AssignedSquawkEvent &event) const {
        const auto fp = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!fp.IsValid()) return;
        if (!fp.GetControllerAssignedData().SetSquawk(event.squawk.c_str())) {
            Logger::Warning("Failed to set squawk {} for {}", event.squawk, event.callsign);
        }
    }

    void MessageService::HandleRunwayMismatchAlertEvent(const RunwayMismatchAlertEvent &event) const {
        m_plugin->Error(std::format(
            "Runway mismatch vs master. Reconfigure EuroScope. Expected DEP {} / ARR {}; current DEP {} / ARR {}.",
            JoinRunways(event.expected_departure),
            JoinRunways(event.expected_arrival),
            JoinRunways(event.current_departure),
            JoinRunways(event.current_arrival)
        ));
    }

    void MessageService::HandleRequestedAltitudeEvent(const RequestedAltitudeEvent &event) const {
        const auto fp = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!fp.IsValid()) return;
        if (!fp.GetControllerAssignedData().SetFinalAltitude(event.altitude)) {
            Logger::Warning("Failed to set request altitude {} for {}", event.altitude, event.callsign);
        }
    }

    void MessageService::HandleClearedAltitudeEvent(const ClearedAltitudeEvent &event) const {
        const auto fp = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!fp.IsValid()) return;
        if (!fp.GetControllerAssignedData().SetClearedAltitude(event.altitude)) {
            Logger::Warning("Failed to set cleared altitude {} for {}", event.altitude, event.callsign);
        }
    }

    void MessageService::HandleCommunicationTypeEvent(const CommunicationTypeEvent &event) const {
        const auto fp = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!fp.IsValid()) return;
        if (event.communication_type.empty()) return;
        if (!fp.GetControllerAssignedData().SetCommunicationType(event.communication_type[0])) {
            Logger::Warning("Failed to set communication type {} for {}", event.communication_type, event.callsign);
        }
    }

    void MessageService::HandleGroundStateEvent(const GroundStateEvent &event) const {
        m_plugin->UpdateViaScratchPad(event.callsign.c_str(), event.ground_state.c_str());
    }

    void MessageService::HandleClearedFlagEvent(const ClearedFlagEvent &event) const {
        const auto* trackedPlan = m_flightPlanService->GetFlightPlan(event.callsign);
        if (!ShouldMirrorClearedFlagToEuroScope(trackedPlan, event.cleared)) {
            return;
        }
        m_plugin->SetClearenceFlag(event.callsign, event.cleared);
    }

    void MessageService::HandleHeadingEvent(const HeadingEvent &event) const {
        const auto fp = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!fp.IsValid()) return;
        if (!fp.GetControllerAssignedData().SetAssignedHeading(event.heading)) {
            Logger::Warning("Failed to set assigned heading {} for {}", event.heading, event.callsign);
        }
    }

    void MessageService::HandleStandEvent(const StandEvent &event) const {
        m_plugin->SetArrivalStand(event.callsign.c_str(), event.stand);
    }

    void MessageService::HandleEobtEvent(const EobtEvent &event) const {
        const auto fp = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!fp.IsValid()) return;
        if (!fp.GetFlightPlanData().SetEstimatedDepartureTime(event.eobt.c_str())) {
            Logger::Warning("Failed to set EOBT {} for {}", event.eobt, event.callsign);
            return;
        }
        if (!fp.GetFlightPlanData().AmendFlightPlan()) {
            Logger::Warning("Failed to amend flight plan {} after EOBT update", event.callsign);
        }
    }

    void MessageService::HandleGenerateSquawkEvent(const GenerateSquawkEvent &event) const {
        const auto fp = m_plugin->FlightPlanSelect(event.callsign.c_str());
        // Automatic requests are routed to the active Delivery client, which
        // can have the flight plan before it has a radar target in range.
        // TopSky generates the squawk from the callsign, so a valid flight plan
        // is sufficient and avoids dropping the one-shot request on spawn.
        if (!fp.IsValid()) return;
        m_plugin->AddNeedsSquawk(std::string(fp.GetCallsign()));
    }

    void MessageService::HandleRouteEvent(const RouteEvent &event) const {
        const auto fp = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!fp.IsValid()) return;
        if (!fp.GetFlightPlanData().SetRoute(event.route.c_str())) {
            Logger::Warning("Failed to set route '{}' for {}", event.route, event.callsign);
        }
        if (!fp.GetFlightPlanData().AmendFlightPlan()) {
            Logger::Warning("Failed to amend flight plan {}", event.callsign);
        }
    }

    void MessageService::HandleRemarksEvent(const RemarksEvent &event) const {
        const auto fp = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!fp.IsValid()) return;
        if (!fp.GetFlightPlanData().SetRemarks(event.remarks.c_str())) {
            Logger::Warning("Failed to set remarks '{}' for {}", event.remarks, event.callsign);
        }
        if (!fp.GetFlightPlanData().AmendFlightPlan()) {
            Logger::Warning("Failed to amend flight plan {}", event.callsign);
        }
    }

    void MessageService::HandleAircraftInfoEvent(const AircraftInfoEvent &event) const {
        const auto fp = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!fp.IsValid()) return;
        if (!fp.GetFlightPlanData().SetAircraftInfo(event.aircraft_type.c_str())) {
            Logger::Warning("Failed to set aircraft info '{}' for {}", event.aircraft_type, event.callsign);
        }
        if (!fp.GetFlightPlanData().AmendFlightPlan()) {
            Logger::Warning("Failed to amend flight plan {}", event.callsign);
        }
    }

    void MessageService::HandleAircraftInfoRemarksEvent(const AircraftInfoRemarksEvent &event) const {
        const auto fp = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!fp.IsValid()) return;
        if (!fp.GetFlightPlanData().SetAircraftInfo(event.aircraft_type.c_str())) {
            Logger::Warning("Failed to set aircraft info '{}' for {}", event.aircraft_type, event.callsign);
        }
        if (!fp.GetFlightPlanData().SetRemarks(event.remarks.c_str())) {
            Logger::Warning("Failed to set remarks '{}' for {}", event.remarks, event.callsign);
        }
        if (!fp.GetFlightPlanData().AmendFlightPlan()) {
            Logger::Warning("Failed to amend flight plan {}", event.callsign);
        }
    }

    void MessageService::HandleSidEvent(const SidEvent &event) const {
        const auto fp = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!fp.IsValid()) return;
        auto route = std::string(fp.GetFlightPlanData().GetRoute());
        m_routeService->SetSid(route, event.sid, m_plugin->GetConnectionState().relevant_airport);
        if (route.empty()) return;
        if (!fp.GetFlightPlanData().SetRoute(route.c_str())) {
            Logger::Warning("Failed to set route '{}' for {}", route, event.callsign);
        }
        if (!fp.GetFlightPlanData().AmendFlightPlan()) {
            Logger::Warning("Failed to amend flight plan {}", event.callsign);
        }
    }

    void MessageService::HandleAircraftRunwayEvent(const AircraftRunwayEvent &event) const {
        const auto fp = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!fp.IsValid()) return;
        // TODO: We only handle departures for now
        const auto airport = m_plugin->GetConnectionState().relevant_airport;
        if (_stricmp(fp.GetFlightPlanData().GetOrigin(), airport.c_str()) != 0) return;
        auto route = std::string(fp.GetFlightPlanData().GetRoute());
        m_routeService->SetDepartureRunway(route, event.runway, airport);
        if (route.empty()) return;
        if (!fp.GetFlightPlanData().SetRoute(route.c_str())) {
            Logger::Warning("Failed to set route '{}' for {}", route, event.callsign);
        }
        if (!fp.GetFlightPlanData().AmendFlightPlan()) {
            Logger::Warning("Failed to amend flight plan {}", event.callsign);
        }

    }

    void MessageService::HandleEsAssumeOnlyEvent(const AssumeOnlyEvent &event) const {
        auto fp = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!fp.IsValid()) {
            Logger::Warning("Failed to find flight plan {} for assume_only", event.callsign);
            return;
        }

        if (fp.GetState() != EuroScopePlugIn::FLIGHT_PLAN_STATE_TRANSFER_TO_ME_INITIATED) {
            Logger::Warning("Flight plan {} is not in state TRANSFER_TO_ME_INITIATED for assume_only", event.callsign);
        }

        fp.AcceptHandoff();
    }

    void MessageService::HandleEsAssumeAndDropEvent(const AssumeAndDropEvent &event) const {
        HandleEsAssumeOnlyEvent(AssumeOnlyEvent{event.callsign});
        HandleEsDropTrackingEvent(DropTrackingEvent{event.callsign});
    }

    void MessageService::HandleEsDropTrackingEvent(const DropTrackingEvent &event) const {
        auto fp = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!fp.IsValid()) {
            Logger::Warning("Failed to find flight plan {} for drop_tracking", event.callsign);
            return;
        }

        if (!fp.EndTracking()) {
            Logger::Warning("Failed to end tracking {} for drop_tracking", event.callsign);
        }
    }

    void MessageService::HandleCoordinationHandoverEvent(const CoordinationHandoverEvent &event) const {
        auto fp = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!fp.IsValid()) {
            Logger::Warning("Failed to find flight plan {} for coordination_handover", event.callsign);
            return;
        }

        auto targetController = m_plugin->ControllerSelect(event.target_callsign.c_str());
        if (!targetController.IsValid() || !targetController.IsController()) {
            Logger::Warning("Failed to find target controller {} for coordination_handover {}", event.target_callsign,
                            event.callsign);
            return;
        }

        if (!fp.GetTrackingControllerIsMe() && !fp.StartTracking()) {
            Logger::Warning("Failed to start tracking {} for coordination_handover", event.callsign);
            return;
        }

        const auto currentHandoffTarget = std::string(fp.GetHandoffTargetControllerCallsign());
        if (!currentHandoffTarget.empty() && _stricmp(currentHandoffTarget.c_str(), event.target_callsign.c_str()) == 0) {
            return;
        }

        if (!fp.InitiateHandoff(event.target_callsign.c_str())) {
            Logger::Warning("Failed to initiate handoff for {} to {}", event.callsign, event.target_callsign);
        }
    }

    void MessageService::HandleBackendSyncEvent(const BackendSyncEvent &event) const {
        m_plugin->SetAirportCoordinates(event.latitude, event.longitude);


        const auto relevantAirport = m_plugin->GetConnectionState().relevant_airport;
        for (const auto &strip : event.strips) {
            const auto fp = m_plugin->FlightPlanSelect(strip.callsign.c_str());
            if (!fp.IsValid()) {
                Logger::Warning("BackendSync: flight plan not found for {}", strip.callsign);
                continue;
            }

            if (!strip.assigned_squawk.empty() &&
                strip.assigned_squawk != fp.GetControllerAssignedData().GetSquawk()) {
                if (!fp.GetControllerAssignedData().SetSquawk(strip.assigned_squawk.c_str())) {
                    Logger::Warning("BackendSync: failed to set squawk {} for {}", strip.assigned_squawk, strip.callsign);
                }
            }

            if (!strip.pdc_state.empty() || !strip.pdc_request_remarks.empty()) {
                m_flightPlanService->ApplyPdcStateChange(strip.callsign, strip.pdc_state, strip.pdc_request_remarks);
            }

            const auto* trackedPlan = m_flightPlanService->GetFlightPlan(strip.callsign);
            const bool desiredCleared = strip.cleared &&
                ShouldMirrorClearedFlagToEuroScope(trackedPlan, strip.cleared);
            if (fp.GetClearenceFlag() != desiredCleared) {
                m_plugin->SetClearenceFlag(strip.callsign, desiredCleared);
            }

            if (!strip.ground_state.empty() && strip.ground_state != fp.GetGroundState()) {
                m_plugin->UpdateViaScratchPad(strip.callsign.c_str(), strip.ground_state.c_str());
            }

            if (!strip.stand.empty() && (trackedPlan == nullptr || trackedPlan->stand != strip.stand)) {
                const auto destination = std::string(fp.GetFlightPlanData().GetDestination());
                if (destination == relevantAirport) {
                    m_plugin->SetArrivalStand(strip.callsign.c_str(), strip.stand);
                }
            }

            m_flightPlanService->ApplyBackendSyncCdm(strip.callsign, strip.cdm);
            m_flightPlanService->ApplyBackendSyncHold(
                strip.callsign, strip.hold, strip.hold_type, strip.hold_eat);
        }
    }

void MessageService::HandlePdcStateChangeEvent(const PdcStateChangeEvent &event) const {
        Logger::Info("PDC state change for {}: {}", event.callsign, event.state);
        m_flightPlanService->ApplyPdcStateChange(event.callsign, event.state, event.pdc_request_remarks);
    }

    void MessageService::HandleSendPrivateMessageEvent(const SendPrivateMessageEvent &event) const {
        Logger::Info("Sending private message to {}", event.callsign);
        PrivateMessageSender::SendPrivateMessage(event.callsign, event.message);
    }

    void MessageService::HandleTrackingControllerChangedEvent(const TrackingControllerChangedEvent& event) const {
        auto flightPlan = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!flightPlan.IsValid() || !m_plugin->IsRelevant(flightPlan)) return;
        // Never replay against a confirmation of another controller's ownership.
        if (_stricmp(flightPlan.GetTrackingControllerCallsign(), event.tracking_controller.c_str()) != 0) return;
        m_flightPlanService->ReplayTrackedHold(flightPlan);

        // All operational clients retain the last authoritative EAT. Apply it
        // now that this client has become the tracker; HOLD_EAT is transient
        // and is not carried into this local TopSky holding list by ownership.
        const auto cached = m_flightPlanService->GetFlightPlan(event.callsign);
        if (cached != nullptr && cached->backend_hold_eat_replay.has_value()) {
            const auto& replay = *cached->backend_hold_eat_replay;
            WriteTopSkyHoldEat(flightPlan, HoldEvent{event.callsign, replay.hold, replay.hold_type, replay.eat});
        }
    }

    void MessageService::HandleHoldEvent(const HoldEvent &event) const {
        // Retain the authoritative value on every operational client. The
        // client that later becomes tracker can then populate its own TopSky
        // holding list without waiting for another AMAN calculation change.
        m_flightPlanService->CacheBackendHoldEatReplay(event.callsign, event.hold, event.hold_type, event.hold_eat);

        auto flightPlan = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!flightPlan.IsValid()) return;
        WriteTopSkyHoldEat(flightPlan, event);
    }

    void MessageService::WriteTopSkyHoldEat(EuroScopePlugIn::CFlightPlan flightPlan, const HoldEvent& event) const {
        if (!flightPlan.IsValid() || !flightPlan.GetTrackingControllerIsMe()) return;

        auto controllerData = flightPlan.GetControllerAssignedData();
        const auto annotation = controllerData.GetFlightStripAnnotation(flightplan::TOPSKY_HOLD_ANNOTATION);
        const auto command = flightplan::BuildTopSkyHoldEatCommand(
            flightplan::ParseTopSkyHoldAnnotation(annotation == nullptr ? "" : annotation), event.hold, event.hold_type, event.hold_eat);
        if (command.empty()) return;

        // Observed backend/TopSky state does not prove that this local TopSky
        // instance received the transient command. Always apply a server replay
        // after validating the live hold.
        m_plugin->UpdateViaScratchPad(event.callsign.c_str(), command.c_str());
    }

    bool MessageService::SendCdmTobtUpdate(const std::string& callsign, const std::string& tobt) const {
        if (!m_webSocketService->IsConnected()) return false;
        m_webSocketService->SendEvent(CdmTobtUpdateEvent(callsign, tobt));
        return true;
    }

    bool MessageService::SendCdmAsrtToggle(const std::string& callsign, const std::string& asrt) const {
        if (!m_webSocketService->IsConnected()) return false;
        m_webSocketService->SendEvent(CdmAsrtToggleEvent(callsign, asrt));
        return true;
    }

    bool MessageService::SendCdmTsacUpdate(const std::string& callsign, const std::string& tsac) const {
        if (!m_webSocketService->IsConnected()) return false;
        m_webSocketService->SendEvent(CdmTsacUpdateEvent(callsign, tsac));
        return true;
    }

    bool MessageService::SendCdmDeiceUpdate(const std::string& callsign, const std::string& deiceType) const {
        if (!m_webSocketService->IsConnected()) return false;
        m_webSocketService->SendEvent(CdmDeiceUpdateEvent(callsign, deiceType));
        return true;
    }

    bool MessageService::SendCdmManualCtot(const std::string& callsign, const std::string& ctot) const {
        if (!m_webSocketService->IsConnected()) return false;
        m_webSocketService->SendEvent(CdmManualCtotEvent(callsign, ctot));
        return true;
    }

    bool MessageService::SendCdmCtotRemove(const std::string& callsign) const {
        if (!m_webSocketService->IsConnected()) return false;
        m_webSocketService->SendEvent(CdmCtotRemoveEvent(callsign));
        return true;
    }

    bool MessageService::SendCdmReady(const std::string& callsign) const {
        if (!m_webSocketService->IsConnected()) return false;
        m_webSocketService->SendEvent(CdmReadyEvent(callsign));
        return true;
    }

    MessageService::CommandOutcome MessageService::HandleCreateFPLEvent(const CreateFPLEvent &event) const {
		using Result = websocket::protobuf::wire::CommandResultEvent;
		const auto fail = [](Result::Reason reason, const char* detail) -> CommandOutcome {
			return {Result::FAILED, reason, detail};
		};
        const auto fp = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!fp.IsValid()) {
            Logger::Warning("create_fpl: flight plan not found for {}", event.callsign);
            return fail(Result::TARGET_NOT_FOUND, "flight plan not found");
        }

        const auto airport = m_plugin->GetConnectionState().relevant_airport;
        auto fpData = fp.GetFlightPlanData();
		bool applied = false;
		const auto set = [&applied, &fail](bool success) -> std::optional<CommandOutcome> {
			if (!success) return fail(applied ? Result::PARTIAL_EXECUTION : Result::EUROSCOPE_API_REJECTED,
			                          "flight plan setter rejected change");
			applied = true;
			return std::nullopt;
		};

        if (!event.origin.empty()) {
            if (auto error = set(fpData.SetOrigin(event.origin.c_str()))) return *error;
        }

        // Set flight rules: "I" for IFR (fpl_type empty), "V" for VFR
        const auto planType = event.fpl_type.empty() ? "I" : "V";
        if (auto error = set(fpData.SetPlanType(planType))) return *error;

        if (!event.destination.empty()) {
            if (auto error = set(fpData.SetDestination(event.destination.c_str()))) return *error;
        }

        if (!event.eobt.empty()) {
            if (auto error = set(fpData.SetEstimatedDepartureTime(event.eobt.c_str()))) return *error;
        }

        if (!event.aircraft_type.empty()) {
            if (auto error = set(fpData.SetAircraftInfo(event.aircraft_type.c_str()))) return *error;
        }

        if (!event.remarks.empty()) {
            if (auto error = set(fpData.SetRemarks(event.remarks.c_str()))) return *error;
        }

        // Build route: start from the event route, then inject SID and departure runway
        auto route = event.route.empty() ? std::string(fpData.GetRoute()) : event.route;

        if (!event.sid.empty()) {
            m_routeService->SetSid(route, event.sid, airport);
        }

        if (!event.runway.empty()) {
            m_routeService->SetDepartureRunway(route, event.runway, airport);
        }

        if (!route.empty()) {
            if (auto error = set(fpData.SetRoute(route.c_str()))) return *error;
        }

        if (!fpData.AmendFlightPlan()) {
            Logger::Warning("create_fpl: failed to amend flight plan for {}", event.callsign);
			return fail(Result::PARTIAL_EXECUTION, "flight plan setters applied but amend failed");
        }

        // Set controller-assigned data after amending the flight plan
        if (!event.assigned_squawk.empty()) {
            if (!fp.GetControllerAssignedData().SetSquawk(event.assigned_squawk.c_str()))
				return fail(Result::PARTIAL_EXECUTION, "flight plan amended but squawk rejected");
        }

        if (event.requested_altitude > 0) {
            if (!fp.GetControllerAssignedData().SetFinalAltitude(event.requested_altitude))
				return fail(Result::PARTIAL_EXECUTION, "flight plan amended but altitude rejected");
        }
		return {Result::EXECUTED, Result::OK, {}};
    }
}
