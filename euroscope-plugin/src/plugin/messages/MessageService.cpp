#include "MessageService.h"

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
    }

    void MessageService::OnMessages(const std::vector<nlohmann::json> &messages) {
        for (const auto &message: messages) {
            HandleMessage(message);
        }
    }

    void MessageService::HandleMessage(const nlohmann::json &message) const {
        const auto type = message["type"].get<std::string>();

        // TODO change to debug
        Logger::Info("Received message: {}", type);

        if (type == EVENT_SESSION_INFO_NAME) {
            HandleSessionInfoEvent(message.get<SessionInfoEvent>());
        } else if (type == EVENT_ASSIGNED_SQUAWK_NAME) {
            HandleAssignedSquawkEvent(message.get<AssignedSquawkEvent>());
        } else if (type == EVENT_REQUESTED_ALTITUDE_NAME) {
            HandleRequestedAltitudeEvent(message.get<RequestedAltitudeEvent>());
        } else if (type == EVENT_CLEARED_ALTITUDE_NAME) {
            HandleClearedAltitudeEvent(message.get<ClearedAltitudeEvent>());
        } else if (type == EVENT_COMMUNICATION_TYPE_NAME) {
            HandleCommunicationTypeEvent(message.get<CommunicationTypeEvent>());
        } else if (type == EVENT_GROUND_STATE_NAME) {
            HandleGroundStateEvent(message.get<GroundStateEvent>());
        } else if (type == EVENT_CLEARED_FLAG_NAME) {
            HandleClearedFlagEvent(message.get<ClearedFlagEvent>());
        } else if (type == EVENT_HEADING_NAME) {
            HandleHeadingEvent(message.get<HeadingEvent>());
        } else if (type == EVENT_STAND_NAME) {
            HandleStandEvent(message.get<StandEvent>());
        } else if (type == EVENT_GENERATE_SQUAWK_NAME) {
            HandleGenerateSquawkEvent(message.get<GenerateSquawkEvent>());
        } else if (type == EVENT_ROUTE_NAME) {
            HandleRouteEvent(message.get<RouteEvent>());
        } else if (type == EVENT_REMARKS_NAME) {
            HandleRemarksEvent(message.get<RemarksEvent>());
        } else if (type == EVENT_SID_NAME) {
            HandleSidEvent(message.get<SidEvent>());
        } else if (type == EVENT_AIRCRAFT_RUNWAY_NAME) {
            HandleAircraftRunwayEvent(message.get<AircraftRunwayEvent>());
        } else if (type == EVENT_COORDINATION_HANDOVER_NAME) {
            HandleCoordinationHandoverEvent(message.get<CoordinationHandoverEvent>());
        } else if (type == EVENT_ASSUME_AND_DROP_NAME) {
            HandleEsAssumeAndDropEvent(message.get<AssumeAndDropEvent>());
        } else if (type == EVENT_BACKEND_SYNC_NAME) {
            HandleBackendSyncEvent(message.get<BackendSyncEvent>());
        } else {
            Logger::Warning("Unknown message type: {}", type);
        }
    }

    void MessageService::HandleSessionInfoEvent(const SessionInfoEvent &event) const {
        const auto state = event.role == "master" ? websocket::STATE_MASTER : websocket::STATE_SLAVE;
        m_webSocketService->SetSessionState(state);

        Logger::Debug("Is master: {}", state == websocket::STATE_MASTER);

        if (state != websocket::STATE_MASTER) {
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
            if (!flightPlanData.IsReceived()) continue;;
            const auto trackPosition = it.GetFPTrackPosition();
            // TODO: is this a problem?
            if (!trackPosition.IsValid()) continue;;
            const auto position = trackPosition.GetPosition();

            const auto callsign = std::string(it.GetCallsign());
            const auto info = m_flightPlanService->GetFlightPlan(callsign);
            const auto isArrival = strcmp(it.GetFlightPlanData().GetDestination(), relevantAirport) == 0;
            const auto runway = std::string(isArrival
                                                ? it.GetFlightPlanData().GetArrivalRwy()
                                                : it.GetFlightPlanData().GetDepartureRwy());
            const auto controllerAssignedData = it.GetControllerAssignedData();
            std::string stand;
            if (info != nullptr) {
                stand = info->stand;
            }

            if (stand.empty() && trackPosition.GetPressureAltitude() < 1000) {
                if (const auto standPtr = m_standService->GetStandFromFlightPlan(it); standPtr != nullptr) {
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
                std::string(flightPlanData.GetRemarks()),
                runway,
                std::string(trackPosition.GetSquawk()),
                std::string(controllerAssignedData.GetSquawk()),
                std::string(flightPlanData.GetSidName()),
                it.GetClearenceFlag(),
                std::string(it.GetGroundState()),
                controllerAssignedData.GetClearedAltitude(),
                flightPlanData.GetFinalAltitude(),
                controllerAssignedData.GetAssignedHeading(),
                std::string(flightPlanData.GetAircraftInfo()),
                {flightPlanData.GetAircraftWtc()},
                Position{
                    position.m_Latitude, position.m_Longitude,
                    trackPosition.GetPressureAltitude()
                },
                stand,
                {flightPlanData.GetCommunicationType()},
                flightPlanData.GetCapibilities() == 0 ? "?" : std::string {flightPlanData.GetCapibilities()},
                isArrival ? "" : std::string(flightPlanData.GetEstimatedDepartureTime()),
                isArrival ? flightplan::FlightPlanService::GetEstimatedLandingTime(it) : "",
                std::string(it.GetTrackingControllerCallsign())
            });
        }

        const auto syncEvent = SyncEvent(strips, controllers, m_runwayService->GetActiveRunways(relevantAirport));
        m_webSocketService->SendEvent(syncEvent);
    }

    void MessageService::HandleAssignedSquawkEvent(const AssignedSquawkEvent &event) const {
        const auto fp = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!fp.IsValid()) return;
        if (!fp.GetControllerAssignedData().SetSquawk(event.squawk.c_str())) {
            Logger::Warning("Failed to set squawk {} for {}", event.squawk, event.callsign);
        }
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
        if (!fp.GetControllerAssignedData().SetCommunicationType(event.communication_type[0])) {
            Logger::Warning("Failed to set communication type {} for {}", event.communication_type, event.callsign);
        }
    }

    void MessageService::HandleGroundStateEvent(const GroundStateEvent &event) const {
        m_plugin->UpdateViaScratchPad(event.callsign.c_str(), event.ground_state.c_str());
    }

    void MessageService::HandleClearedFlagEvent(const ClearedFlagEvent &event) const {
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

    void MessageService::HandleGenerateSquawkEvent(const GenerateSquawkEvent &event) const {
        const auto fp = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!fp.IsValid() || !fp.GetCorrelatedRadarTarget().IsValid()) return;
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

    void MessageService::HandleEsAssumeAndDropEvent(const AssumeAndDropEvent &event) const {
        auto fp = m_plugin->FlightPlanSelect(event.callsign.c_str());
        if (!fp.IsValid()) {
            Logger::Warning("Failed to find flight plan {} for assume_and_drop", event.callsign);
            return;
        }

        if (fp.GetState() != EuroScopePlugIn::FLIGHT_PLAN_STATE_TRANSFER_TO_ME_INITIATED) {
            Logger::Warning("Flight plan {} is not in state TRANSFER_TO_ME_INITIATED for assume_and_drop", event.callsign);
        }

        fp.AcceptHandoff();

        if (!fp.EndTracking()) {
            Logger::Warning("Failed to end tracking {} for assume_and_drop", event.callsign);
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

            if (!strip.assigned_squawk.empty()) {
                if (!fp.GetControllerAssignedData().SetSquawk(strip.assigned_squawk.c_str())) {
                    Logger::Warning("BackendSync: failed to set squawk {} for {}", strip.assigned_squawk, strip.callsign);
                }
            }

            m_plugin->SetClearenceFlag(strip.callsign, strip.cleared);

            if (!strip.ground_state.empty()) {
                m_plugin->UpdateViaScratchPad(strip.callsign.c_str(), strip.ground_state.c_str());
            }

            if (!strip.stand.empty()) {
                const auto destination = std::string(fp.GetFlightPlanData().GetDestination());
                if (destination == relevantAirport) {
                    m_plugin->SetArrivalStand(strip.callsign.c_str(), strip.stand);
                }
            }
        }
    }
}
