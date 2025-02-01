#pragma once
#include "Events.h"
#include "WebSocket.h"
#include "authentication/AuthenticationService.h"
#include "configuration/AppConfig.h"
#include "handlers/TimedEventHandler.h"
#include "plugin/FlightStripsPlugin.h"

namespace FlightStrips::websocket {
    class WebSocketService final : public handlers::TimedEventHandler {
    public:
        explicit WebSocketService(const std::shared_ptr<configuration::AppConfig> &appConfig,
                                  const std::shared_ptr<authentication::AuthenticationService> &authentication_service,
                                  const std::shared_ptr<FlightStripsPlugin> &plugin);

        ~WebSocketService() override;

        void OnTimer(int time) override;

        template<typename T> requires std::is_base_of_v<Event, T>
        void SendEvent(const T &event);

    private:
        std::shared_ptr<configuration::AppConfig> m_appConfig;
        std::shared_ptr<authentication::AuthenticationService> m_authentication_service;
        std::shared_ptr<FlightStripsPlugin> m_plugin;
        WebSocket webSocket;
        std::string primary;

        static void OnMessage(const std::string &message);
        void OnConnected();
        void SendLoginEvent();
    };
}
