//
// Created by fsr19 on 04/07/2023.
//

#include <nlohmann/json.hpp>
#include "MessageHandler.h"
#include "plugin/FlightStripsPlugin.h"

using json = nlohmann::json;

FlightStrips::network::MessageHandler::MessageHandler(const std::shared_ptr<FlightStripsPlugin> &mPlugin) : m_plugin(
        mPlugin) {}

void FlightStrips::network::MessageHandler::OnMessage(const std::string& string) {
    json j = json::parse(string);
    if (!j.contains("$type")) {
        return;
    }

    auto type = j["$type"];

    try {
        if (type == "Initial") {
            this->m_plugin->Information(j["message"]);
            return;
        } else if (type == "SetSquawk") {
            if (j.contains("callsign") && j.contains("squawk")) {
                auto plan = this->m_plugin->FlightPlanSelect(j["callsign"].get_ref<std::string&>().c_str());
                if (!plan.IsValid()) return;
                plan.GetControllerAssignedData().SetSquawk(to_string(j["squawk"]).c_str());
            }
            return;
        } else if (type == "SetFinalAltitude") {
            if (j.contains("callsign") && j.contains("altitude")) {
                auto plan = this->m_plugin->FlightPlanSelect(j["callsign"].get_ref<std::string&>().c_str());
                if (!plan.IsValid()) return;
                plan.GetFlightPlanData().SetFinalAltitude(j["altitude"]);
            }
            return;
        } else if (type == "SetClearedAltitude") {
            if (j.contains("callsign") && j.contains("altitude")) {
                auto plan = this->m_plugin->FlightPlanSelect(j["callsign"].get_ref<std::string&>().c_str());
                if (!plan.IsValid()) return;
                plan.GetControllerAssignedData().SetClearedAltitude(j["altitude"]);
            }
            return;

        } else if (type == "SetCommunicationType") {
            if (j.contains("callsign") && j.contains("communicationType")) {
                auto plan = this->m_plugin->FlightPlanSelect(j["callsign"].get_ref<std::string&>().c_str());
                if (!plan.IsValid()) return;
                plan.GetControllerAssignedData().SetCommunicationType(j["communicationType"].get_ref<std::string&>().front());
            }
            return;
        } else if (type == "SetCleared") {
            if (j.contains("callsign") && j.contains("cleared")) {
                this->m_plugin->SetClearenceFlag(j["callsign"], j["cleared"]);
            }
            return;
        }

    } catch(std::exception& e) {
        this->m_plugin->Information(e.what());
        this->m_plugin->Information("Failed event: " + j.dump());

    }

    this->m_plugin->Information("Unknown event: " + j.dump());
}
