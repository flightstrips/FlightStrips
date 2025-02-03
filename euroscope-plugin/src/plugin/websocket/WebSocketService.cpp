#include "WebSocketService.h"

#include <nlohmann/json.hpp>

#include "Events.h"
#include "Logger.h"

namespace FlightStrips::websocket {
    WebSocketService::WebSocketService(const std::shared_ptr<configuration::AppConfig> &appConfig,
                                       const std::shared_ptr<authentication::AuthenticationService> &
                                       authentication_service,
                                       const std::shared_ptr<FlightStripsPlugin> &plugin,
                                       const std::shared_ptr<handlers::ConnectionEventHandlers> & event_handlers,
                                       const std::shared_ptr<handlers::MessageHandlers>& message_handlers) :
                                                         m_appConfig(appConfig),
                                                         m_authentication_service(authentication_service),
                                                         m_plugin(plugin),
                                                         m_connection_handlers(event_handlers),
                                                         m_messageHandlers(message_handlers),
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
                                        CONNECTION_TYPE_DIRECT || state.connection_type == CONNECTION_TYPE_PLAYBACK) &&
                                    m_authentication_service->GetAuthenticationState() == authentication::AUTHENTICATED;
        if (should_connect && (webSocket.GetStatus() == WEBSOCKET_STATUS_DISCONNECTED || webSocket.GetStatus() ==
                               WEBSOCKET_STATUS_FAILED)) {
            primary = state.primary_frequency;
            Logger::Info("Trying to connect to server: {}", m_appConfig->GetBaseUrl());
            webSocket.Connect();
            return;
        }

        if (!should_connect && IsConnected()) {
            Logger::Info("Disconnecting from server: {}", m_appConfig->GetBaseUrl());
            webSocket.Disconnect();
            std::lock_guard lock(message_mutex_);
            messages_.clear();
            client_state = STATE_UNKNOWN;
            return;
        }

        if (!should_connect || !IsConnected()) return;

        if (primary != state.primary_frequency) {
            SendLoginEvent();
        }

        std::vector<nlohmann::json> messages;
        {
            std::lock_guard lock(message_mutex_);
            if (messages_.empty()) return;
            Logger::Info("Got messages {} from server", messages_.size());
            messages = std::move(messages_);
        }

        m_messageHandlers->OnMessages(messages);
    }

    bool WebSocketService::IsConnected() const {
        return webSocket.GetStatus() == WEBSOCKET_STATUS_CONNECTED;
    }

    bool WebSocketService::ShouldSend() const {
        return IsConnected() && client_state == STATE_MASTER;
    }

    void WebSocketService::SetSessionState(const ClientState state) {
        client_state = state;
    }

    template<typename T> requires std::is_base_of_v<Event, T>
    void WebSocketService::SendEvent(const T &event) {
        const nlohmann::json json = event;
        const auto json_str = json.dump();
        webSocket.Send(json_str);
        Logger::Debug("Sending event: {}", json_str);
    }

    void WebSocketService::OnMessage(const std::string &message) {
        const auto json = nlohmann::json::parse(message, nullptr, false, false);
        if (json.is_discarded()) {
            Logger::Warning("Invalid JSON message: {}", message);
            return;
        }

        std::lock_guard lock(message_mutex_);
        messages_.push_back(json);
    }

    void WebSocketService::OnConnected() {
        const auto token = TokenEvent(m_authentication_service->GetAccessToken());
        SendEvent(token);
        SendLoginEvent();

        m_connection_handlers->OnOnline();
    }

    void WebSocketService::SendLoginEvent() {
        const auto &state = m_plugin->GetConnectionState();
        primary = state.primary_frequency;
        const auto login = LoginEvent(state.relevant_airport, state.primary_frequency, state.callsign, state.range);
        SendEvent(login);
    }
}
