#pragma once
#include "Events.h"
#include "WebSocket.h"
#include "authentication/AuthenticationService.h"
#include "configuration/AppConfig.h"
#include "handlers/TimedEventHandler.h"

namespace FlightStrips::websocket {
    class WebSocketService : public handlers::TimedEventHandler {
    public:
        explicit WebSocketService(const std::shared_ptr<configuration::AppConfig> &appConfig,
                                  const std::shared_ptr<authentication::AuthenticationService> &authentication_service);

        ~WebSocketService() override;

        void OnTimer(int time);

        void Start();

        void Stop();

        template<typename T> requires std::is_base_of_v<Event, T>
        void SendEvent(const T &event);

    private:
        std::shared_ptr<authentication::AuthenticationService> authentication_service;
        WebSocket webSocket;

        static void OnMessage(const std::string &message);
        void OnConnected();
    };
}
