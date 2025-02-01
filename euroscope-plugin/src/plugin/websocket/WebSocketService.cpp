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
                                       authentication_service,
                                       const std::shared_ptr<FlightStripsPlugin> &plugin) : m_appConfig(appConfig),
        m_authentication_service(authentication_service),
        m_plugin(plugin),
        webSocket(
            appConfig->GetBaseUrl(),
            [this](const std::string &message) {
                this->OnMessage(message);
            }, [this] { this->OnConnected(); }) {
    }

    WebSocketService::~WebSocketService() {
    }

    void WebSocketService::OnTimer(int time) {
        const auto &state = m_plugin->GetConnectionState();
        const bool should_connect = !state.primary_frequency.empty() && state.primary_frequency != "199.998" && !state.
                                    relevant_airport.empty() && (
                                        state.connection_type == CONNECTION_TYPE_SWEATBOX || state.connection_type ==
                                        CONNECTION_TYPE_DIRECT || state.connection_type == CONNECTION_TYPE_PLAYBACK);
        if (should_connect && (webSocket.GetStatus() == WEBSOCKET_STATUS_DISCONNECTED || webSocket.GetStatus() ==
                               WEBSOCKET_STATUS_FAILED)) {
            primary = state.primary_frequency;
            Logger::Info("Trying to connect to server: {}", m_appConfig->GetBaseUrl());
            webSocket.Connect();
            return;
        }

        if (!should_connect && webSocket.GetStatus() == WEBSOCKET_STATUS_CONNECTED) {
            Logger::Info("Disconnecting from server: {}", m_appConfig->GetBaseUrl());
            webSocket.Disconnect();
            return;
        }

        if (!should_connect) return;

        if (webSocket.GetStatus() == WEBSOCKET_STATUS_CONNECTED && primary != state.primary_frequency) {
            SendLoginEvent();
        }
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
        const auto token = TokenEvent(m_authentication_service->GetAccessToken());
        SendEvent(token);
        SendLoginEvent();
    }

    void WebSocketService::SendLoginEvent() {
        const auto &state = m_plugin->GetConnectionState();
        primary = state.primary_frequency;
        const auto login = LoginEvent(state.relevant_airport, state.primary_frequency, state.callsign, state.range);
        SendEvent(login);
    }
}
