#pragma once
#include "Events.h"
#include "Logger.h"
#include "WebSocket.h"
#include "authentication/AuthenticationService.h"
#include "configuration/AppConfig.h"
#include "handlers/AuthenticationEventHandler.h"
#include "handlers/ConnectionEventHandlers.h"
#include "handlers/MessageHandlers.h"
#include "handlers/TimedEventHandler.h"
#include "plugin/FlightStripsPlugin.h"

namespace FlightStrips::websocket {
    enum ClientState {
        STATE_UNKNOWN,
        STATE_SLAVE,
        STATE_MASTER
    };

    struct Stats {
        int tx = 0;
        int rx = 0;
    };

    class WebSocketService final : public handlers::TimedEventHandler, public handlers::AuthenticationEventHandler {
    public:
        explicit WebSocketService(const std::shared_ptr<configuration::AppConfig> &appConfig,
                                  const std::shared_ptr<authentication::AuthenticationService> &authentication_service,
                                  const std::shared_ptr<FlightStripsPlugin> &plugin,
                                  const std::shared_ptr<handlers::ConnectionEventHandlers> &event_handlers,
                                  const std::shared_ptr<handlers::MessageHandlers> &message_handlers);

        ~WebSocketService() override;

        void OnTimer(int time) override;
        void OnTokenUpdate(const std::string &token) override;

        template<typename T> requires std::is_base_of_v<Event, T>
        void SendEvent(const T &event);
        bool IsConnected() const;
        bool ShouldSend() const;
        void SetSessionState(ClientState state);
        Stats GetStats() const;

    private:
        std::shared_ptr<configuration::AppConfig> m_appConfig;
        std::shared_ptr<authentication::AuthenticationService> m_authentication_service;
        std::shared_ptr<FlightStripsPlugin> m_plugin;
        std::shared_ptr<handlers::ConnectionEventHandlers> m_connection_handlers;
        std::shared_ptr<handlers::MessageHandlers> m_messageHandlers;
        WebSocket webSocket;
        std::string primary;
        ClientState client_state = STATE_UNKNOWN;

        std::mutex message_mutex_;
        std::vector<nlohmann::json> messages_ {};

        int tx;
        int rx;

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
    webSocket.Send(json_str);
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
template void FlightStrips::websocket::WebSocketService::SendEvent<StripUpdateEvent>(const StripUpdateEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<SyncEvent>(const SyncEvent & event);
