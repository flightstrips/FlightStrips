#pragma once
#include <chrono>
#include <optional>
#include "Events.h"
#include "Logger.hpp"
#include "WebSocket.h"
#include "authentication/IAuthenticationService.h"
#include "configuration/AppConfig.h"
#include "handlers/AuthenticationEventHandler.h"
#include "handlers/ConnectionEventHandlers.h"
#include "handlers/MessageHandlers.h"
#include "handlers/TimedEventHandler.h"
#include "plugin/IFlightStripsPlugin.h"

namespace FlightStrips::websocket {
    enum ClientState {
        STATE_UNKNOWN,
        STATE_SLAVE,
        STATE_MASTER
    };

    struct Stats {
        int tx = 0;
        int rx = 0;
        int queued = 0;
        ClientState role = STATE_UNKNOWN;
    };

    class WebSocketService : public handlers::TimedEventHandler, public handlers::AuthenticationEventHandler {
    public:
        explicit WebSocketService(const std::shared_ptr<configuration::AppConfig> &appConfig,
                                  const std::shared_ptr<authentication::IAuthenticationService> &authentication_service,
                                  const std::shared_ptr<IFlightStripsPlugin> &plugin,
                                  const std::shared_ptr<handlers::ConnectionEventHandlers> &event_handlers,
                                  const std::shared_ptr<handlers::MessageHandlers> &message_handlers);

        ~WebSocketService() override;

        void OnTimer(int time) override;
        void OnTokenUpdate(const std::string &token) override;

        template<typename T> requires std::is_base_of_v<Event, T>
        void SendEvent(const T &event);
        bool IsConnected() const;
        bool IsPendingConnect() const;
        bool ShouldSend() const;
        void SetSessionState(ClientState state);
        Stats GetStats() const;
        std::optional<int> GetDelaySecondsRemaining() const;

    protected:
        // Test seam: construct with a pre-built WebSocket (e.g. wrapping a mock ImplBase).
        WebSocketService(const std::shared_ptr<authentication::IAuthenticationService> &authentication_service,
                         const std::shared_ptr<IFlightStripsPlugin> &plugin,
                         const std::shared_ptr<handlers::ConnectionEventHandlers> &event_handlers,
                         const std::shared_ptr<handlers::MessageHandlers> &message_handlers,
                         std::unique_ptr<WebSocket> webSocket,
                         bool enabled);

    private:
        std::shared_ptr<configuration::AppConfig> m_appConfig;
        std::shared_ptr<authentication::IAuthenticationService> m_authentication_service;
        std::shared_ptr<IFlightStripsPlugin> m_plugin;
        std::shared_ptr<handlers::ConnectionEventHandlers> m_connection_handlers;
        std::shared_ptr<handlers::MessageHandlers> m_messageHandlers;
        std::unique_ptr<WebSocket> webSocket;
        std::string primary;
        ClientState client_state = STATE_UNKNOWN;

        mutable std::mutex message_mutex_;
        std::vector<nlohmann::json> messages_ {};

        bool enabled;

        static constexpr int CONNECT_DELAY_SECONDS = 30;
        static constexpr int FAST_CONNECT_DELAY_SECONDS = 5;
        std::optional<std::chrono::steady_clock::time_point> connect_after_;
        bool pending_connect_ = false;
        bool has_been_offline_ = false;

        int tx = 0;
        int rx = 0;

        void OnMessage(const std::string &message);
        void OnConnected();
        void SendLoginEvent();
    };
}

template<typename T> requires std::is_base_of_v<Event, T>
void FlightStrips::websocket::WebSocketService::SendEvent(const T &event) {
    ++tx;
    const nlohmann::json json = event;
    const auto json_str = json.dump();
    webSocket->Send(json_str);
    Logger::Debug("Sending event: {}", json_str);
}

template void FlightStrips::websocket::WebSocketService::SendEvent<AircraftDisconnectEvent>(const AircraftDisconnectEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<AssignedSquawkEvent>(const AssignedSquawkEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<ClearedAltitudeEvent>(const ClearedAltitudeEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<ClearedFlagEvent>(const ClearedFlagEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<CommunicationTypeEvent>(const CommunicationTypeEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<ControllerOfflineEvent>(const ControllerOfflineEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<ControllerOnlineEvent>(const ControllerOnlineEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<GroundStateEvent>(const GroundStateEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<HeadingEvent>(const HeadingEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<PositionEvent>(const PositionEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<RequestedAltitudeEvent>(const RequestedAltitudeEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<RunwayEvent>(const RunwayEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<SquawkEvent>(const SquawkEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<StandEvent>(const StandEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<TrackingControllerChangedEvent>(const TrackingControllerChangedEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<StripUpdateEvent>(const StripUpdateEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<SyncEvent>(const SyncEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<CoordinationReceivedEvent>(const CoordinationReceivedEvent & event);
