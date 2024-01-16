#pragma once

#include "NetworkService.h"

namespace FlightStrips {
    class FlightStripsPlugin;
    namespace network {
        class ConnectedClient;
    }
    struct Container;
}

namespace FlightStrips::network {
    class MessageHandler {
    public:
        MessageHandler(Container& mContainer, ConnectedClient *mConnectedClient);
        ~MessageHandler();

        void OnMessage(const std::string& string);

    private:
       Container &m_container;
       ConnectedClient* m_connectedClient;

    };
}
