#pragma once

#include "NetworkService.h"

namespace FlightStrips {
    class FlightStripsPlugin;
    namespace network {
        class ConnectedClient;
    }
}

namespace FlightStrips::network {
    class MessageHandler {
    public:
        MessageHandler(const std::shared_ptr<FlightStripsPlugin>& mPlugin, ConnectedClient *mConnectedClient);

        void OnMessage(const std::string& string);

    private:
       std::shared_ptr<FlightStripsPlugin> m_plugin;
       ConnectedClient* m_connectedClient;

    };
}
