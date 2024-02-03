//
// Created by fsr19 on 04/07/2023.
//

#include <nlohmann/json.hpp>
#include "MessageHandler.h"
#include "plugin/FlightStripsPlugin.h"
#include "ConnectedClient.h"
#include "bootstrap/Container.h"

using json = nlohmann::json;

FlightStrips::network::MessageHandler::MessageHandler(Container& mContainer, ConnectedClient *mConnectedClient) : m_container(
        mContainer), m_connectedClient(mConnectedClient) {}

FlightStrips::network::MessageHandler::~MessageHandler() {
    this->m_connectedClient = nullptr;
}

void FlightStrips::network::MessageHandler::OnMessage(const std::string& string) {
    json j = json::parse(string);
    if (!j.contains("$type")) {
        return;
    }

    auto plugin = this->m_container.plugin;
    auto type = j["$type"];

    try {
        if (type == "Initial") {
            // Client is fully ready send everything
            auto connectionType = plugin->GetConnectionType();
            auto me = plugin->ControllerMyself();
            this->m_container.networkService->ConnectionTypeUpdate(connectionType, me);
            plugin->OnAirportRunwayActivityChanged();

            // Don't do anything else if not connected.
            if (connectionType == 0) return;

            plugin->OnControllerPositionUpdate(me);



            for (auto plan = plugin->FlightPlanSelectFirst(); plan.IsValid(); plan = plugin->FlightPlanSelectNext(plan)) {
                try {
                    plugin->OnFlightPlanFlightPlanDataUpdate(plan);
                    plugin->OnRadarTargetPositionUpdate(plan.GetCorrelatedRadarTarget());
                    plugin->OnFlightPlanControllerAssignedDataUpdate(plan, EuroScopePlugIn::CTR_DATA_TYPE_SQUAWK);
                    plugin->OnFlightPlanControllerAssignedDataUpdate(plan, EuroScopePlugIn::CTR_DATA_TYPE_FINAL_ALTITUDE);
                    plugin->OnFlightPlanControllerAssignedDataUpdate(plan, EuroScopePlugIn::CTR_DATA_TYPE_TEMPORARY_ALTITUDE);
                    plugin->OnFlightPlanControllerAssignedDataUpdate(plan, EuroScopePlugIn::CTR_DATA_TYPE_COMMUNICATION_TYPE);
                    plugin->OnFlightPlanControllerAssignedDataUpdate(plan, EuroScopePlugIn::CTR_DATA_TYPE_SCRATCH_PAD_STRING);
                    plugin->OnFlightPlanControllerAssignedDataUpdate(plan, EuroScopePlugIn::CTR_DATA_TYPE_GROUND_STATE);
                    plugin->OnFlightPlanControllerAssignedDataUpdate(plan, EuroScopePlugIn::CTR_DATA_TYPE_CLEARENCE_FLAG);
                } catch (std::exception &e) {
                    plugin->Error("Failed to send plan (" + std::string(plan.GetCallsign()) + "):" + std::string(e.what()));
                }
            }

            for (auto controller = plugin->ControllerSelectFirst(); controller.IsValid(); controller = plugin->ControllerSelectNext(controller)) {
                try {
                    plugin->OnControllerPositionUpdate(controller);
                } catch (std::exception &e) {
                    plugin->Error("Failed to send controller (" + std::string(controller.GetCallsign()) + "):" + std::string(e.what()));
                }
            }

            return;
        } else if (type == "SetSquawk") {
            if (j.contains("callsign") && j.contains("squawk")) {
                auto plan = plugin->FlightPlanSelect(j["callsign"].get_ref<std::string&>().c_str());
                if (!plan.IsValid()) return;
                plan.GetControllerAssignedData().SetSquawk(to_string(j["squawk"]).c_str());
            }
            return;
        } else if (type == "SetFinalAltitude") {
            if (j.contains("callsign") && j.contains("altitude")) {
                auto plan = plugin->FlightPlanSelect(j["callsign"].get_ref<std::string&>().c_str());
                if (!plan.IsValid()) return;
                plan.GetFlightPlanData().SetFinalAltitude(j["altitude"]);
            }
            return;
        } else if (type == "SetClearedAltitude") {
            if (j.contains("callsign") && j.contains("altitude")) {
                auto plan = plugin->FlightPlanSelect(j["callsign"].get_ref<std::string&>().c_str());
                if (!plan.IsValid()) return;
                plan.GetControllerAssignedData().SetClearedAltitude(j["altitude"]);
            }
            return;

        } else if (type == "SetCommunicationType") {
            if (j.contains("callsign") && j.contains("communicationType")) {
                auto plan = plugin->FlightPlanSelect(j["callsign"].get_ref<std::string&>().c_str());
                if (!plan.IsValid()) return;
                plan.GetControllerAssignedData().SetCommunicationType(j["communicationType"].get_ref<std::string&>().front());
            }
            return;
        } else if (type == "SetGroundState") {
            if (j.contains("callsign") && j.contains("state")) {
                plugin->UpdateViaScratchPad(
                        j["callsign"].get_ref<std::string&>().c_str(),
                        j["state"].get_ref<std::string&>().c_str());
            }
            return;
        } else if (type == "SetCleared") {
            if (j.contains("callsign") && j.contains("cleared")) {
                plugin->SetClearenceFlag(j["callsign"], j["cleared"]);
            }
            return;
        } else if (type == "SetFlightPlanRoute") {
            if (j.contains("callsign") && j.contains("route")) {
                auto plan = plugin->FlightPlanSelect(j["callsign"].get_ref<std::string&>().c_str());
                if (!plan.IsValid()) return;
                plan.GetFlightPlanData().SetRoute(j["route"].get_ref<std::string&>().c_str());
            }
            return;
        } else if (type == "SetRemarks") {
            if (j.contains("callsign") && j.contains("remarks")) {
                auto plan = plugin->FlightPlanSelect(j["callsign"].get_ref<std::string&>().c_str());
                if (!plan.IsValid()) return;
                plan.GetFlightPlanData().SetRemarks(j["remarks"].get_ref<std::string&>().c_str());
            }
            return;
        } else if (type == "SetDepartureRunway") {
            if (j.contains("callsign") && j.contains("runway")) {
                auto plan = plugin->FlightPlanSelect(j["callsign"].get_ref<std::string&>().c_str());
                if (!plan.IsValid()) return;
                // TODO
            }
            return;
        } else if (type == "SetSID") {
            if (j.contains("callsign") && j.contains("sid")) {
                auto plan = plugin->FlightPlanSelect(j["callsign"].get_ref<std::string&>().c_str());
                if (!plan.IsValid()) return;
                // TODO
            }
            return;
        } else if (type == "Me") {
            auto controller = plugin->ControllerMyself();
            auto connectionType = plugin->GetConnectionType();

            auto data = json{
                    { "$type", "ControllerMe", },
                    { "isMe", true },
                    { "connectionType", std::to_string(connectionType) },
            };

            if (controller.IsValid()) {
                data["frequency"] = controller.GetPrimaryFrequency();
                data["position"] = controller.GetPositionId();
                data["callsign"] = controller.GetCallsign();
            }
            this->m_connectedClient->Write(data.dump());
            return;
        }
    } catch(std::exception& e) {
        plugin->Information(e.what());
        plugin->Information("Failed event: " + j.dump());
    }

    plugin->Information("Unknown event: " + j.dump());
}
