//
// Created by fsr19 on 26/05/2023.
//

#pragma once

#include "ConnectedClient.h"

namespace FlightStrips {
    namespace network {

        class Server {
        public:
            Server();
            ~Server();

            std::vector<std::string> ReadMessages();

            void SendMessage(const std::string& message);
        private:
            void ListenLoop();



            SOCKET m_ServerSocket = INVALID_SOCKET;
            bool isActive = true;

            std::unique_ptr<std::thread> m_ListenThread;
            std::vector<std::unique_ptr<ConnectedClient>> m_Clients;
        };

    } // FlightStrips
} // network

