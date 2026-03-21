#include "WebSocketService.h"

#include <nlohmann/json.hpp>

#include "Events.h"
#include "Logger.hpp"

namespace FlightStrips::websocket {
    WebSocketService::WebSocketService(const std::shared_ptr<configuration::AppConfig> &appConfig,
                                       const std::shared_ptr<authentication::IAuthenticationService> &
                                       authentication_service,
                                       const std::shared_ptr<IFlightStripsPlugin> &plugin,
                                       const std::shared_ptr<handlers::ConnectionEventHandlers> & event_handlers,
                                       const std::shared_ptr<handlers::MessageHandlers>& message_handlers) :
                                                         m_appConfig(appConfig),
                                                         m_authentication_service(authentication_service),
                                                         m_plugin(plugin),
                                                         m_connection_handlers(event_handlers),
                                                         m_messageHandlers(message_handlers),
                                                         webSocket(std::make_unique<WebSocket>(
                                                             appConfig->GetBaseUrl(),
                                                             [this](const std::string &message) {
                                                                 this->OnMessage(message);
                                                             }, [this] { this->OnConnected(); })) {
        enabled = appConfig->GetApiEnabled();
    }

    WebSocketService::WebSocketService(const std::shared_ptr<authentication::IAuthenticationService> &authentication_service,
                                       const std::shared_ptr<IFlightStripsPlugin> &plugin,
                                       const std::shared_ptr<handlers::ConnectionEventHandlers> &event_handlers,
                                       const std::shared_ptr<handlers::MessageHandlers> &message_handlers,
                                       std::unique_ptr<WebSocket> ws,
                                       const bool enabled) :
                                                         m_authentication_service(authentication_service),
                                                         m_plugin(plugin),
                                                         m_connection_handlers(event_handlers),
                                                         m_messageHandlers(message_handlers),
                                                         webSocket(std::move(ws)),
                                                         enabled(enabled) {
    }

    WebSocketService::~WebSocketService() {
    }

    void WebSocketService::OnTimer(int time) {
        if (!enabled) return;
        const auto &state = m_plugin->GetConnectionState();

        const bool freq_ok = !state.primary_frequency.empty() && state.primary_frequency != "199.998";
        const bool airport_ok = !state.relevant_airport.empty();
        const bool conn_ok = state.connection_type == CONNECTION_TYPE_SWEATBOX || state.connection_type ==
                             CONNECTION_TYPE_DIRECT || state.connection_type == CONNECTION_TYPE_PLAYBACK;
        const bool auth_ok = m_authentication_service->GetAuthenticationState() == authentication::AUTHENTICATED
                          || m_authentication_service->GetAuthenticationState() == authentication::REFRESH;
        const bool should_connect = freq_ok && airport_ok && conn_ok && auth_ok;

        if (!should_connect && IsConnected()) {
            Logger::Warning("Disconnecting from server — reason: freq_ok={} (freq='{}') airport_ok={} (airport='{}') conn_ok={} (type={}) auth_ok={}",
                freq_ok, state.primary_frequency,
                airport_ok, state.relevant_airport,
                conn_ok, static_cast<int>(state.connection_type),
                auth_ok);
        }

        if (should_connect && (webSocket->GetStatus() == WEBSOCKET_STATUS_DISCONNECTED || webSocket->GetStatus() ==
                               WEBSOCKET_STATUS_FAILED)) {
            const auto now = std::chrono::steady_clock::now();
            if (!connect_after_.has_value()) {
                const auto delay = has_been_offline_ ? CONNECT_DELAY_SECONDS : FAST_CONNECT_DELAY_SECONDS;
                connect_after_ = now + std::chrono::seconds(delay);
                pending_connect_ = true;
                Logger::Info("Detected online, waiting {}s before connecting to server", delay);
                return;
            }
            if (now < connect_after_.value()) {
                return;
            }
            connect_after_.reset();
            primary = state.primary_frequency;
            Logger::Info("Trying to connect to server: {}", m_appConfig->GetBaseUrl());
            webSocket->Connect();
            return;
        }

        if (!should_connect && IsConnected()) {
            webSocket->Disconnect();
            std::lock_guard lock(message_mutex_);
            messages_.clear();
            client_state = STATE_UNKNOWN;
            connect_after_.reset();
            pending_connect_ = false;
            has_been_offline_ = true;
            return;
        }

        if (!should_connect || !IsConnected()) {
            if (!should_connect) {
                connect_after_.reset();
                pending_connect_ = false;
                has_been_offline_ = true;
            }
            return;
        }

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

    void WebSocketService::OnTokenUpdate(const std::string &token) {
        if (!IsConnected()) return;
        const auto event = TokenEvent(token);
        SendEvent(event);
    }

    bool WebSocketService::IsConnected() const {
        return webSocket->GetStatus() == WEBSOCKET_STATUS_CONNECTED;
    }

    bool WebSocketService::IsPendingConnect() const {
        return pending_connect_ || webSocket->GetStatus() == WEBSOCKET_STATUS_CONNECTING;
    }

    bool WebSocketService::ShouldSend() const {
        return IsConnected() && client_state == STATE_MASTER;
    }

    void WebSocketService::SetSessionState(const ClientState state) {
        client_state = state;
    }

    Stats WebSocketService::GetStats() const {
        std::lock_guard lock(message_mutex_);
        return Stats{tx, rx, static_cast<int>(messages_.size()), client_state};
    }

    std::optional<int> WebSocketService::GetDelaySecondsRemaining() const {
        if (!connect_after_.has_value()) return std::nullopt;
        const auto remaining = std::chrono::duration_cast<std::chrono::seconds>(
            connect_after_.value() - std::chrono::steady_clock::now()).count();
        if (remaining <= 0) return std::nullopt;
        return static_cast<int>(remaining);
    }



    void WebSocketService::OnMessage(const std::string &message) {
        rx++;
        const auto json = nlohmann::json::parse(message, nullptr, false, false);
        if (json.is_discarded()) {
            Logger::Warning("Invalid JSON message: {}", message);
            return;
        }

        std::lock_guard lock(message_mutex_);
        messages_.push_back(json);
    }

    void WebSocketService::OnConnected() {
        tx = 0;
        rx = 0;
        pending_connect_ = false;
        const auto token = TokenEvent(m_authentication_service->GetAccessToken());
        SendEvent(token);
        SendLoginEvent();

        m_connection_handlers->OnOnline();
    }

    void WebSocketService::SendLoginEvent() {
        const auto &[range, connection_type, primary_frequency, callsign, relevant_airport] = m_plugin->GetConnectionState();
        primary = primary_frequency;

        const auto connection = connection_type == CONNECTION_TYPE_DIRECT ? "LIVE" : connection_type == CONNECTION_TYPE_SWEATBOX ? "SWEATBOX" : "PLAYBACK";
        const auto login = LoginEvent(relevant_airport, connection, primary_frequency, callsign, range);
        SendEvent(login);
    }
}
