#pragma once
#include "WebSocket.h"
#include "configuration/AppConfig.h"
#include "handlers/TimedEventHandler.h"

namespace FlightStrips::websocket {
    class WebSocketService : public handlers::TimedEventHandler{
    public:
        explicit WebSocketService(const std::shared_ptr<configuration::AppConfig> &appConfig);
        ~WebSocketService() override;

        void OnTimer(int time);
        void Start();
        void Stop();

    private:
        WebSocket webSocket;

        static void OnMessage(const std::string &message);
    };
}
