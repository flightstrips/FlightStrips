//
// Created by fsr19 on 25/05/2023.
//

#include <winsock.h>
#include "ConnectedClient.h"
#include "bootstrap/Container.h"

namespace FlightStrips::network {
    ConnectedClient::ConnectedClient(SOCKET socket, Container& mContainer)
            : socket(socket), m_messageHandler(mContainer, this) {
        this->isActive = true;
        this->writerThread = std::make_unique<std::thread>(&ConnectedClient::WriteLoop, this);
        this->readerThread = std::make_unique<std::thread>(&ConnectedClient::ReadLoop, this);
    }

    ConnectedClient::~ConnectedClient() {
        if (this->isActive) {
            this->isActive = false;
            // TODO check result
            shutdown(this->socket, SD_BOTH);
        }

        closesocket(this->socket);
        this->readerThread->join();
        this->writerSemaphore.release();
        this->writerThread->join();
    }

    void ConnectedClient::Write(const std::string& message) {
        this->writerMutex.lock();

        this->writeQueue.push(message);

        this->writerSemaphore.release();
        this->writerMutex.unlock();
    }

    void ConnectedClient::ReadLoop() {

        int bytesReceived = 0;
        std::array<char, 4096> receiveBuffer{};

        std::array<char, 4096> messageBuffer{};
        int index = 0;

        while (this->isActive) {
            bytesReceived = recv(this->socket, &receiveBuffer[0], READ_BUFFER_SIZE, 0);

            if (bytesReceived > 0)  {
                for (int i = 0; i < bytesReceived; i++) {
                    char byte = receiveBuffer[i];

                    if (byte == 0) {
                        auto string = std::string(messageBuffer.cbegin(), messageBuffer.cbegin() + index);
                        m_messageHandler.OnMessage(string);
                        /*
                        auto lock = std::lock_guard(this->readerMutex);

                        this->readQueue.emplace();
                         */
                        index = 0;
                        continue;
                    }

                    messageBuffer[index++] = byte;
                }
            } else {
                this->isActive = false;
            }
        }

    }

    void ConnectedClient::WriteLoop() {
        while (this->isActive) {

            this->writerSemaphore.acquire();

            if (!this->isActive) break;

            this->writerMutex.lock();

            auto message = this->writeQueue.front();
            this->writeQueue.pop();

            this->writerMutex.unlock();

            message.push_back(0);

            int sendResult = send(this->socket, message.c_str(), static_cast<int>(message.size()), 0);
            if (sendResult == SOCKET_ERROR) {
                this->isActive = false;
            }
        }
    }

    bool ConnectedClient::IsActive() const {
        return this->isActive;
    }
}

