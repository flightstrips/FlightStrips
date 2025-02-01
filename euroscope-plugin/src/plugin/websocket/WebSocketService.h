#pragma once
#include "Events.h"
#include "WebSocket.h"
#include "authentication/AuthenticationService.h"
#include "configuration/AppConfig.h"
#include "handlers/ConnectionEventHandlers.h"
#include "handlers/TimedEventHandler.h"
#include "plugin/FlightStripsPlugin.h"

namespace FlightStrips::websocket {
    class WebSocketService final : public handlers::TimedEventHandler {
    public:
        explicit WebSocketService(const std::shared_ptr<configuration::AppConfig> &appConfig,
                                  const std::shared_ptr<authentication::AuthenticationService> &authentication_service,
                                  const std::shared_ptr<FlightStripsPlugin> &plugin,
                                  const std::shared_ptr<handlers::ConnectionEventHandlers> &event_handlers);

        ~WebSocketService() override;

        void OnTimer(int time) override;

        template<typename T> requires std::is_base_of_v<Event, T>
        void SendEvent(const T &event);
        bool IsConnected() const;

    private:
        std::shared_ptr<configuration::AppConfig> m_appConfig;
        std::shared_ptr<authentication::AuthenticationService> m_authentication_service;
        std::shared_ptr<FlightStripsPlugin> m_plugin;
        std::shared_ptr<handlers::ConnectionEventHandlers> m_connection_handlers;
        WebSocket webSocket;
        std::string primary;

        static void OnMessage(const std::string &message);
        void OnConnected();
        void SendLoginEvent();
    };
}

template void FlightStrips::websocket::WebSocketService::SendEvent<RunwayEvent>(const RunwayEvent & event);
