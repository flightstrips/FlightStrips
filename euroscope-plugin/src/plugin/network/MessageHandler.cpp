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

    if (j["$type"] == "Initial") {
        this->m_plugin->Information(j["message"]);
    }
}
