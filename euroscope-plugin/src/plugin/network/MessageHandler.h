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
        MessageHandler(const std::shared_ptr<Container>& mContainer, ConnectedClient *mConnectedClient);

        void OnMessage(const std::string& string);

    private:
       std::shared_ptr<Container> m_container;
       ConnectedClient* m_connectedClient;

    };
}
