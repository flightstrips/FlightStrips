#include "NetworkService.h"
#include "euroscope/JsonConversion.hpp"
#include "stands/StandService.h"

namespace FlightStrips::network {
    NetworkService::NetworkService(const std::shared_ptr<FlightStrips::network::Server> &server, const std::shared_ptr<FlightStrips::stands::StandService> &standService)
        : m_server(server), m_standService(standService) {
    }

    void NetworkService::FlightPlanEvent(EuroScopePlugIn::CFlightPlan flightPlan) {
        json j = json{};
        euroscope::to_json(j, flightPlan.GetFlightPlanData());
        j["callsign"] = flightPlan.GetCallsign();
        j["$type"] = "FlightPlanUpdated";

        auto stand = this->m_standService->GetStandFromFlightPlan(flightPlan);

        if (stand == nullptr) {
            j["stand"] = "";
        } else {
            j["stand"] = stand->GetName();
        }

        auto message = j.dump();

        this->m_server->SendMessage(message);
    }

    void NetworkService::ControllerFlightPlanDataEvent(EuroScopePlugIn::CFlightPlan flightPlan,
                                                                              int dataType) {
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
        auto data = json{
            { "$type", "FlightPlanDisconnected" },
            { "callsign", flightPlan.GetCallsign() }
        };

        this->m_server->SendMessage(data.dump());
    }

    void NetworkService::SquawkUpdateEvent(std::string callsign, int squawk) {
        auto data = json{
                { "$type", "SquawkUpdate" },
                { "callsign", callsign },
                { "squawk", squawk }
        };

        this->m_server->SendMessage(data.dump());
    }

    void NetworkService::SendActiveRunways(std::vector<runway::ActiveRunway> &runways) const {
        auto arr = json::array();

        for (auto it = runways.begin(); it != runways.end(); ++it) {
            auto element = json{
                    { "name", it->name },
                    { "isDeparture", it->isDeparture }
            };

            arr.push_back(element);
        }

        auto data = json{
                { "$type", "ActiveRunways"},
                { "runways", arr}
        };

        this->m_server->SendMessage(data.dump());
    }

    void NetworkService::ControllerPositionUpdateEvent(EuroScopePlugIn::CController controller) {
        auto data = json{
                { "$type", "ControllerUpdate"},
                { "callsign", controller.GetCallsign() },
                { "frequency", controller.GetPrimaryFrequency() },
                { "position", controller.GetPositionId() }
        };

        this->m_server->SendMessage(data.dump());
    }

    void NetworkService::ControllerDisconnectEvent(EuroScopePlugIn::CController controller) {
        auto data = json{
                { "$type", "ControllerDisconnect"},
                { "callsign", controller.GetCallsign() },
                { "frequency", controller.GetPrimaryFrequency() },
        };

        this->m_server->SendMessage(data.dump());
    }
}

