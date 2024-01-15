//
// Created by fsr19 on 26/05/2023.
//

#pragma once

namespace FlightStrips {
    struct Container;
    namespace network {
        class ConnectedClient;
    }
}

namespace FlightStrips {
    namespace network {

        class Server {
        public:
            explicit Server(const std::shared_ptr<Container>& mContainer);
            ~Server();

            void SendMessage(const std::string& message);
        private:
            void ListenLoop();



            SOCKET m_ServerSocket = INVALID_SOCKET;
            bool isActive = true;

            std::unique_ptr<std::thread> m_ListenThread;
            std::vector<std::unique_ptr<ConnectedClient>> m_Clients;
            std::shared_ptr<Container> m_container;
        };

    } // FlightStrips
} // network

