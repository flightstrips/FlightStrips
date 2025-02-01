#include "FlightPlanService.h"

namespace FlightStrips::flightplan {
    FlightPlanService::FlightPlanService(
        const std::shared_ptr<websocket::WebSocketService> &websocketService,
        const std::shared_ptr<FlightStripsPlugin> &flightStripsPlugin) : m_websocketService(websocketService),
                                                                         m_flightStripsPlugin(flightStripsPlugin),
                                                                         m_flightPlans({}) {
    }

    void FlightPlanService::RadarTargetPositionEvent(EuroScopePlugIn::CRadarTarget radarTarget) {
        const auto position = radarTarget.GetPosition();
        if (!position.IsValid()) return;
        FlightPlan plan = {std::string(position.GetSquawk())};
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
        m_websocketService->SendEvent(PositionEvent(callsign, aircraftPosition.m_Latitude, aircraftPosition.m_Longitude,
                                                    position.GetPressureAltitude()));
    }

    void FlightPlanService::FlightPlanEvent(EuroScopePlugIn::CFlightPlan flightPlan) {
        if (!m_websocketService->ShouldSend()) return;
        const auto isArrival = strcmp(flightPlan.GetFlightPlanData().GetDestination(),
                                      m_flightStripsPlugin->GetConnectionState().relevant_airport.c_str()) == 0;
        const auto event = StripUpdateEvent(
            std::string(flightPlan.GetCallsign()),
            std::string(flightPlan.GetFlightPlanData().GetOrigin()),
            std::string(flightPlan.GetFlightPlanData().GetDestination()),
            std::string(flightPlan.GetFlightPlanData().GetAlternate()),
            std::string(flightPlan.GetFlightPlanData().GetRoute()),
            std::string(flightPlan.GetFlightPlanData().GetRemarks()),
            std::string(isArrival
                            ? flightPlan.GetFlightPlanData().GetArrivalRwy()
                            : flightPlan.GetFlightPlanData().GetDepartureRwy()),
            std::string(flightPlan.GetFlightPlanData().GetSidName()),
            std::string(flightPlan.GetFlightPlanData().GetAircraftInfo()),
            {flightPlan.GetFlightPlanData().GetAircraftWtc()},
            {flightPlan.GetFlightPlanData().GetCapibilities()},
            isArrival ? "" : std::string(flightPlan.GetFlightPlanData().GetEstimatedDepartureTime()),
            isArrival ? GetEstimatedLandingTime(flightPlan) : ""
        );
        m_websocketService->SendEvent(event);
    }

    void FlightPlanService::ControllerFlightPlanDataEvent(EuroScopePlugIn::CFlightPlan flightPlan, int dataType) {
        if (!m_websocketService->ShouldSend()) return;
        const auto callsign = std::string(flightPlan.GetCallsign());

        switch (dataType) {
            case EuroScopePlugIn::CTR_DATA_TYPE_SQUAWK:
                m_websocketService->SendEvent(
                    AssignedSquawkEvent(callsign, std::string(flightPlan.GetControllerAssignedData().GetSquawk())));
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_FINAL_ALTITUDE:
                m_websocketService->SendEvent(
                    RequestedAltitudeEvent(callsign, flightPlan.GetControllerAssignedData().GetFinalAltitude()));
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_TEMPORARY_ALTITUDE:
                m_websocketService->SendEvent(
                    ClearedAltitudeEvent(callsign, flightPlan.GetControllerAssignedData().GetClearedAltitude()));
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_COMMUNICATION_TYPE:
                m_websocketService->SendEvent(
                    CommunicationTypeEvent(callsign, flightPlan.GetControllerAssignedData().GetCommunicationType()));
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_GROUND_STATE:
                // TODO maybe get the ground state from topsky instead
                m_websocketService->SendEvent(GroundStateEvent(callsign, std::string(flightPlan.GetGroundState())));
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_CLEARENCE_FLAG:
                m_websocketService->SendEvent(ClearedFlagEvent(callsign, flightPlan.GetClearenceFlag()));
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_HEADING:
                m_websocketService->SendEvent(
                    HeadingEvent(callsign, flightPlan.GetControllerAssignedData().GetAssignedHeading()));
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
        m_websocketService->SendEvent(AircraftDisconnectEvent(std::string(flightPlan.GetCallsign())));
    }

    std::string FlightPlanService::GetEstimatedLandingTime(const EuroScopePlugIn::CFlightPlan& flightPlan) {
        time_t rawtime;
        tm ptm;

        time(&rawtime);
        rawtime += flightPlan.GetPositionPredictions().GetPointsNumber() * 60;
        gmtime_s(&ptm, &rawtime);

        return std::format("{:0>2}{:0>2}", ptm.tm_hour, ptm.tm_min);
    }
}
