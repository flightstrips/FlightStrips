#include "NetworkService.h"
#include "euroscope/JsonConversion.hpp"

namespace FlightStrips::network {
    NetworkService::NetworkService(const std::shared_ptr<FlightStrips::network::Server> &server)
        : m_server(server) {
    }

    void NetworkService::FlightPlanEvent(EuroScopePlugIn::CFlightPlan flightPlan) {
        if (!IsRelevant(flightPlan)) {
            return;
        }

        json j = json{};
        euroscope::to_json(j, flightPlan.GetFlightPlanData());
        j["callsign"] = flightPlan.GetCallsign();
        j["$type"] = "FlightPlanUpdated";
        auto message = j.dump();

        this->m_server->SendMessage(message);
    }

    void NetworkService::ControllerFlightPlanDataEvent(EuroScopePlugIn::CFlightPlan flightPlan,
                                                                              int dataType) {
        if (!IsRelevant(flightPlan)) {
            return;
        }

        auto data = json{
                { "$type", "ControllerDataUpdated" },
                { "callsign", flightPlan.GetCallsign()}
        };

        switch (dataType) {
            case EuroScopePlugIn::CTR_DATA_TYPE_SQUAWK:
                data["type"] = "squawk";
                data["data"] = flightPlan.GetControllerAssignedData().GetSquawk();
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_FINAL_ALTITUDE:
                data["type"] = "final_altitude";
                data["data"] = flightPlan.GetControllerAssignedData().GetFinalAltitude();
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_TEMPORARY_ALTITUDE:
                data["type"] = "cleared_altitude";
                data["data"] = flightPlan.GetControllerAssignedData().GetClearedAltitude();
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_COMMUNICATION_TYPE:
                data["type"] = "communication_type";
                data["data"] = euroscope::toCharString(flightPlan.GetControllerAssignedData().GetCommunicationType());
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_GROUND_STATE:
                data["type"] = "ground_state";
                data["data"] = flightPlan.GetGroundState();
                break;
            case EuroScopePlugIn::CTR_DATA_TYPE_CLEARENCE_FLAG:
                data["type"] = "clearence_flag";
                data["data"] = flightPlan.GetClearenceFlag();
                break;
            default:
                return;
        }

        this->m_server->SendMessage(data.dump());
    }

    void NetworkService::FlightPlanDisconnectEvent(EuroScopePlugIn::CFlightPlan flightPlan) {
        if (!IsRelevant(flightPlan)) {
            return;
        }

        auto data = json{
            { "$type", "FlightPlanDisconnected" },
            { "callsign", flightPlan.GetCallsign() }
        };

        this->m_server->SendMessage(data.dump());
    }

    bool NetworkService::IsRelevant(EuroScopePlugIn::CFlightPlan flightPlan) {
        return strcmp(flightPlan.GetFlightPlanData().GetDestination(), AIRPORT) == 0
            || strcmp(flightPlan.GetFlightPlanData().GetOrigin(), AIRPORT) == 0;
    }
}

