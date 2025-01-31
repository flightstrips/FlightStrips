//
// Created by fsr19 on 18/01/2025.
//

#include "WebSocketService.h"

#include <nlohmann/json.hpp>

#include "Events.h"
#include "Logger.h"

namespace FlightStrips::websocket {
    WebSocketService::WebSocketService(const std::shared_ptr<configuration::AppConfig> &appConfig,
                                       const std::shared_ptr<authentication::AuthenticationService> &
                                       authentication_service) : authentication_service(authentication_service),
                                                                 webSocket(
                                                                     appConfig->GetBaseUrl(),
                                                                     [this](const std::string &message) {
                                                                         this->OnMessage(message);
                                                                     }, [this] { this->OnConnected(); }) {
    }

    WebSocketService::~WebSocketService() {
    }

    void WebSocketService::OnTimer(int time) {
        if (!webSocket.IsConnected()) {
            //Logger::Info("Starting websocket connection");
            //webSocket.Connect();
            return;
        }

        webSocket.Send(std::format("EuroScope time: {}", time));
    }

    void WebSocketService::Start() {
        webSocket.Connect();
    }

    void WebSocketService::Stop() {
        webSocket.Disconnect();
    }

    template<typename T> requires std::is_base_of_v<Event, T>
    void WebSocketService::SendEvent(const T &event) {
        const nlohmann::json json = event;
        webSocket.Send(json.dump());
    }

    void WebSocketService::OnMessage(const std::string &message) {
        const auto json = nlohmann::json::parse(message, nullptr, false, false);
        if (json.is_discarded()) {
            Logger::Warning("Invalid JSON message: {}", message);
            return;
        }
    }

    void WebSocketService::OnConnected() {
        const auto token = TokenEvent(authentication_service->GetAccessToken());
        SendEvent(token);
    }
}
