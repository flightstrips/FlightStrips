//
// Created by fsr19 on 26/05/2023.
//

#include "Server.h"
#include "ConnectedClient.h"
#include "bootstrap/Container.h"
#include "plugin/FlightStripsPlugin.h"

namespace FlightStrips {
    namespace network {
        Server::Server(const std::shared_ptr<Container>& mContainer) : m_container(mContainer) {
            struct addrinfo *addressInfo = nullptr, hints{};
            ZeroMemory(&hints, sizeof(hints));
            hints.ai_family = AF_INET;
            hints.ai_socktype = SOCK_STREAM;
            hints.ai_protocol = IPPROTO_TCP;
            hints.ai_flags = AI_PASSIVE;

            // Resolve the local address and port to be used by the server
            int getResult = getaddrinfo(nullptr, "27015", &hints, &addressInfo);
            if (getResult != 0) {
                //LogError("getaddrinfo failed when started integration server: " + std::to_string(getResult));
                return;
            }

            // Create the socket
            this->m_ServerSocket = ::socket(addressInfo->ai_family, addressInfo->ai_socktype, addressInfo->ai_protocol);
            if (this->m_ServerSocket == INVALID_SOCKET) {
                //LogError("Failed to initialise integration server socket: " + std::to_string(WSAGetLastError()));
                return;
            }

            // Bind it
            int bindResult = bind(this->m_ServerSocket, addressInfo->ai_addr, static_cast<int>(addressInfo->ai_addrlen));

            // Free the struct that we don't need anymore
            freeaddrinfo(addressInfo);

            // Check the binding
            if (bindResult == SOCKET_ERROR) {
                //LogError("Failed to bind integration server socket: " + std::to_string(WSAGetLastError()));
                closesocket(this->m_ServerSocket);
                return;
            }

            // Listen on the socket
            if (listen(this->m_ServerSocket, SOMAXCONN) == SOCKET_ERROR) {
                //LogError("Failed to listen integration server socket: " + std::to_string(WSAGetLastError()));
                closesocket(this->m_ServerSocket);
                return;
            }

            this->m_ListenThread = std::make_unique<std::thread>(&Server::ListenLoop, this);
        }

        Server::~Server() {
            this->isActive = false;
            if (this->m_ServerSocket != INVALID_SOCKET) {
                closesocket(this->m_ServerSocket);
                this->m_ServerSocket = INVALID_SOCKET;
            }
            this->m_ListenThread->join();
        }

        void Server::ListenLoop() {

            while (this->isActive) {
                SOCKET client  = accept(this->m_ServerSocket, nullptr, nullptr);
                if (client == INVALID_SOCKET) {
                    closesocket(client);
                    continue;
                }

                this->m_container->plugin->Information("Client connected");
                this->m_Clients.push_back(std::make_unique<ConnectedClient>(client, this->m_container));
            }

        }

        void Server::SendMessage(const std::string& message) {
            auto it = this->m_Clients.begin();

            while (it != this->m_Clients.end())
            {
                if (!(*it)->IsActive()) {
                    this->m_container->plugin->Information("Client no longer active");
                    it = this->m_Clients.erase(it);
                } else {
                    (*it)->Write(message);
                    ++it;
                }
            }
        }

    } // FlightStrips
} // network