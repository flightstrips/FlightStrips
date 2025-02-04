#include "MessageService.h"

#include "Logger.h"

namespace FlightStrips::messages {
    void MessageService::OnMessages(const std::vector<nlohmann::json> &messages) {
        for (const auto &message: messages) {
            HandleMessage(message);
        }
    }

    void MessageService::HandleMessage(const nlohmann::json &message) const {
        const auto type = message["type"].get<std::string>();

        if (type == EVENT_SESSION_INFO_NAME) {
            HandleSessionInfoEvent(message.get<SessionInfoEvent>());
        } else if (type == EVENT_ASSIGNED_SQUAWK_NAME) {
            HandleAssignedSquawkEvent(message.get<AssignedSquawkEvent>());
        } else if (type == EVENT_REQUESTED_ALTITUDE_NAME) {
            HandleRequestedAltitudeEvent(message.get<RequestedAltitudeEvent>());
        } else if (type == EVENT_CLEARED_ALTITUDE_NAME) {
            HandleClearedAltitudeEvent(message.get<ClearedAltitudeEvent>());
        } else {
            Logger::Warning("Unknown message type: {}", type);
        }
    }

    void MessageService::HandleSessionInfoEvent(const SessionInfoEvent &event) const {
        const auto state = event.role == "master" ? websocket::STATE_MASTER : websocket::STATE_SLAVE;
        m_webSocketService->SetSessionState(state);

        Logger::Debug("Is master: {}", state == websocket::STATE_MASTER);

        if (state != websocket::STATE_MASTER) return;

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
                controllerAssignedData.GetFinalAltitude(),
                controllerAssignedData.GetAssignedHeading(),
                std::string(flightPlanData.GetAircraftInfo()),
                {flightPlanData.GetAircraftWtc()},
                Position{
                    position.m_Latitude, position.m_Longitude,
                    trackPosition.GetPressureAltitude()
                },
                stand,
                {flightPlanData.GetCommunicationType()},
                {flightPlanData.GetCapibilities()},
                isArrival ? "" : std::string(flightPlanData.GetEstimatedDepartureTime()),
                isArrival ? flightplan::FlightPlanService::GetEstimatedLandingTime(it) : ""
            });
        }

        const auto syncEvent = SyncEvent(strips, controllers);
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
}
