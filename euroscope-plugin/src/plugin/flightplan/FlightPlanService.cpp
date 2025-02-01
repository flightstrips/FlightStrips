#include "FlightPlanService.h"

namespace FlightStrips::flightplan {
    FlightPlanService::FlightPlanService(std::shared_ptr<websocket::WebSocketService> websocketService) : m_websocketService(websocketService), m_flightPlans({}) {
    }

    void FlightPlanService::RadarTargetPositionEvent(EuroScopePlugIn::CRadarTarget radarTarget) {
        const auto position = radarTarget.GetPosition();
        if (!position.IsValid()) return;
        FlightPlan plan = { std::string(position.GetSquawk()) };
        const auto callsign = std::string(radarTarget.GetCallsign());

        const auto [pair, exists] = this->m_flightPlans.insert({callsign, plan});
        bool shouldSendSquawkEvent = true;

        if (!exists) {
            if (pair->second.squawk == plan.squawk) {
                shouldSendSquawkEvent = false;
            } else {
                pair->second.squawk = plan.squawk;

            }
        }
        if (!m_websocketService->ShouldSend()) return;
        if (shouldSendSquawkEvent) m_websocketService->SendEvent(SquawkEvent(callsign, plan.squawk));
        const auto aircraftPosition = position.GetPosition();
        m_websocketService->SendEvent(PositionEvent(callsign, aircraftPosition.m_Latitude, aircraftPosition.m_Longitude, position.GetPressureAltitude()));
    }

    void FlightPlanService::FlightPlanEvent(EuroScopePlugIn::CFlightPlan flightPlan) {
        if (!m_websocketService->ShouldSend()) return;
    }

    void FlightPlanService::ControllerFlightPlanDataEvent(EuroScopePlugIn::CFlightPlan flightPlan, int dataType) {
        if (!m_websocketService->ShouldSend()) return;
        const auto callsign = std::string(flightPlan.GetCallsign());

        switch (dataType) {
            case EuroScopePlugIn::CTR_DATA_TYPE_SQUAWK:
                m_websocketService->SendEvent(AssignedSquawkEvent(callsign, std::string(flightPlan.GetControllerAssignedData().GetSquawk())));
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_FINAL_ALTITUDE:
                m_websocketService->SendEvent(RequestedAltitudeEvent(callsign, flightPlan.GetControllerAssignedData().GetFinalAltitude()));
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_TEMPORARY_ALTITUDE:
                m_websocketService->SendEvent(ClearedAltitudeEvent(callsign, flightPlan.GetControllerAssignedData().GetClearedAltitude()));
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_COMMUNICATION_TYPE:
                m_websocketService->SendEvent(CommunicationTypeEvent(callsign, flightPlan.GetControllerAssignedData().GetCommunicationType()));
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_GROUND_STATE:
                // TODO maybe get the ground state from topsky instead
                m_websocketService->SendEvent(GroundStateEvent(callsign, std::string(flightPlan.GetGroundState())));
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_CLEARENCE_FLAG:
                m_websocketService->SendEvent(ClearedFlagEvent(callsign, flightPlan.GetClearenceFlag()));
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_HEADING:
                m_websocketService->SendEvent(HeadingEvent(callsign, flightPlan.GetControllerAssignedData().GetAssignedHeading()));
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_DEPARTURE_SEQUENCE:
                // TODO should we use this???
                break;
            default:
                break;
        }
    }

    void FlightPlanService::FlightPlanDisconnectEvent(EuroScopePlugIn::CFlightPlan flightPlan) {
        if (!m_websocketService->ShouldSend()) return;
    }
}
