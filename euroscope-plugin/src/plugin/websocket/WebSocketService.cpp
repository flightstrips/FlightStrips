//
// Created by fsr19 on 18/01/2025.
//

#include "WebSocketService.h"

#include <nlohmann/json.hpp>

#include "Logger.h"

namespace FlightStrips::websocket {
    WebSocketService::WebSocketService(const std::shared_ptr<configuration::AppConfig> &appConfig) : webSocket(
        appConfig->GetBaseUrl(), [this](const std::string &message) { this->OnMessage(message); }) {
    }

    WebSocketService::~WebSocketService() {
        Stop();
    }

    void WebSocketService::OnTimer(int time) {
        if (!webSocket.IsConnected()) {
            Logger::Info("Starting websocket connection");
            webSocket.Start();
            return;
        }

        webSocket.Send(std::format("EuroScope time: {}", time));
    }

    void WebSocketService::Start() {
        webSocket.Start();
    }

    void WebSocketService::Stop() {
        webSocket.Stop();
    }

    void WebSocketService::OnMessage(const std::string &message) {
        const auto json = nlohmann::json::parse(message, nullptr, false, false);
        if (json.is_discarded()) {
            Logger::Warning("Invalid JSON message: {}", message);
            return;
        }
    }
}
