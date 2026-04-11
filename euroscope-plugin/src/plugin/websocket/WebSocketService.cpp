#include "WebSocketService.h"

#include <nlohmann/json.hpp>

#include "ExceptionHandling.h"
#include "Events.h"
#include "Logger.hpp"

namespace FlightStrips::websocket {
    WebSocketService::WebSocketService(std::string baseUrl,
                                       const bool apiEnabled,
                                       const std::shared_ptr<authentication::IAuthenticationService> &
                                       authentication_service,
                                       const std::shared_ptr<IFlightStripsPlugin> &plugin,
                                       const std::shared_ptr<handlers::ConnectionEventHandlers> & event_handlers,
                                       const std::shared_ptr<handlers::MessageHandlers>& message_handlers) :
                                                         m_base_url(baseUrl),
                                                         m_authentication_service(authentication_service),
                                                         m_plugin(plugin),
                                                         m_connection_handlers(event_handlers),
                                                         m_messageHandlers(message_handlers),
                                                         webSocket(std::make_unique<WebSocket>(
                                                             std::move(baseUrl),
                                                             [this](const std::string &message) {
                                                                 this->OnMessage(message);
                                                             }, [this] { this->OnConnected(); })),
                                                         enabled(apiEnabled) {
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
        const auto now = std::chrono::steady_clock::now();

        const bool freq_ok = !state.primary_frequency.empty() && state.primary_frequency != "199.998";
        const bool airport_ok = !state.relevant_airport.empty();
        const bool conn_ok = state.connection_type == CONNECTION_TYPE_SWEATBOX || state.connection_type ==
                             CONNECTION_TYPE_DIRECT || state.connection_type == CONNECTION_TYPE_PLAYBACK;
        const bool auth_ok = m_authentication_service->GetAuthenticationState() == authentication::AUTHENTICATED
                          || m_authentication_service->GetAuthenticationState() == authentication::REFRESH;
        const bool should_connect = freq_ok && airport_ok && conn_ok && auth_ok;

        // Track how long we've been online (network+airport+auth) but without a primary frequency.
        // If a primary is selected after ≥30s in this state, skip the connect delay.
        const bool online_but_no_freq = !freq_ok && airport_ok && conn_ok && auth_ok;
        const auto online_without_primary_elapsed = online_without_primary_since_.has_value()
            ? std::chrono::duration_cast<std::chrono::seconds>(now - online_without_primary_since_.value()).count()
            : 0LL;
        const bool skip_delay_for_primary = should_connect &&
            online_without_primary_elapsed >= ONLINE_WITHOUT_PRIMARY_THRESHOLD_SECONDS;

        if (online_but_no_freq && !IsConnected()) {
            if (!online_without_primary_since_.has_value())
                online_without_primary_since_ = now;
        } else {
            online_without_primary_since_.reset();
        }

        if (!should_connect && IsConnected()) {
            Logger::Warning("Disconnecting from server — reason: freq_ok={} (freq='{}') airport_ok={} (airport='{}') conn_ok={} (type={}) auth_ok={}",
                freq_ok, state.primary_frequency,
                airport_ok, state.relevant_airport,
                conn_ok, static_cast<int>(state.connection_type),
                auth_ok);
        }

        if (should_connect && (webSocket->GetStatus() == WEBSOCKET_STATUS_DISCONNECTED || webSocket->GetStatus() ==
                               WEBSOCKET_STATUS_FAILED)) {
            if (skip_delay_for_primary)
                connect_readiness_ = ConnectReadiness::RECONNECT;

            if (!connect_after_.has_value()) {
                pending_connect_ = true;
                if (webSocket->GetStatus() == WEBSOCKET_STATUS_FAILED) {
                    const auto delay_ms = BACKOFF_MS[std::min(fail_count_, static_cast<int>(BACKOFF_MS.size()) - 1)];
                    fail_count_++;
                    connect_after_ = now + std::chrono::milliseconds(delay_ms);
                    Logger::Info("Connect failed (attempt {}), retrying in {}ms", fail_count_, delay_ms);
                    return;
                }
                if (connect_readiness_ == ConnectReadiness::RECONNECT) {
                    Logger::Info("Connecting immediately");
                } else {
                    connect_after_ = now + std::chrono::seconds(FAST_CONNECT_DELAY_SECONDS);
                    Logger::Info("Detected online, waiting {}s before connecting to server", FAST_CONNECT_DELAY_SECONDS);
                    return;
                }
            }
            if (connect_after_.has_value() && now < connect_after_.value()) {
                return;
            }
            connect_after_.reset();
            primary = state.primary_frequency;
            Logger::Info("Trying to connect to server: {}", m_base_url);
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
            fail_count_ = 0;
            connect_readiness_ = ConnectReadiness::RECONNECT;
            return;
        }

        if (!should_connect || !IsConnected()) {
            if (!should_connect) {
                connect_after_.reset();
                pending_connect_ = false;
                fail_count_ = 0;
                connect_readiness_ = ConnectReadiness::RECONNECT;
            }
            return;
        }

        if (!session_name.empty() && session_name != GetEffectiveSessionName(state)) {
            Logger::Info("Session mode changed: '{}' -> '{}', reconnecting", session_name, GetEffectiveSessionName(state));
            Reconnect();
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
        exceptions::RunGuarded("WebSocketService::OnTokenUpdate", [this, &token] {
            if (!IsConnected()) return;
            const auto event = TokenEvent(token);
            SendEvent(event);
        });
    }

    bool WebSocketService::IsConnected() const {
        return webSocket->GetStatus() == WEBSOCKET_STATUS_CONNECTED;
    }

    bool WebSocketService::IsPendingConnect() const {
        return pending_connect_ || webSocket->GetStatus() == WEBSOCKET_STATUS_CONNECTING;
    }

    bool WebSocketService::IsBackingOff() const {
        return fail_count_ > 0 && connect_after_.has_value();
    }

    bool WebSocketService::ShouldSend() const {
        return IsConnected() && client_state == STATE_MASTER;
    }

    void WebSocketService::Reconnect() {
        connect_after_.reset();
        pending_connect_ = false;
        fail_count_ = 0;
        connect_readiness_ = ConnectReadiness::RECONNECT;
        primary.clear();
        session_name.clear();

        if (IsConnected()) {
            webSocket->Disconnect();
        }

        std::lock_guard lock(message_mutex_);
        messages_.clear();
        client_state = STATE_UNKNOWN;
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
        exceptions::RunGuarded("WebSocketService::OnMessage", [this, &message] {
            rx++;
            const auto json = nlohmann::json::parse(message, nullptr, false, false);
            if (json.is_discarded()) {
                Logger::Warning("Invalid JSON message: {}", message);
                return;
            }

            std::lock_guard lock(message_mutex_);
            messages_.push_back(json);
        });
    }

    void WebSocketService::OnConnected() {
        exceptions::RunGuarded("WebSocketService::OnConnected", [this] {
            tx = 0;
            rx = 0;
            pending_connect_ = false;
            fail_count_ = 0;
            connect_after_.reset();
            connect_readiness_ = ConnectReadiness::RECONNECT;
            const auto token = TokenEvent(m_authentication_service->GetAccessToken());
            SendEvent(token);
            SendLoginEvent();

            m_connection_handlers->OnOnline();
        });
    }

    void WebSocketService::SendLoginEvent() {
        const auto& state = m_plugin->GetConnectionState();
        primary = state.primary_frequency;
        session_name = GetEffectiveSessionName(state);

        const auto login = LoginEvent(state.relevant_airport, session_name, state.primary_frequency, state.callsign, state.range);
        SendEvent(login);
    }
}
