//
// Created by fsr19 on 25/05/2023.
//

#include "ConnectedClient.h"

namespace FlightStrips::network {
    ConnectedClient::ConnectedClient(SOCKET socket) : socket(socket) {
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

    std::string ConnectedClient::Read() {
        if (!HasMessage()) return {};

        std::string message;

        this->readerMutex.lock();

        message = this->readQueue.front();
        this->readQueue.pop();

        this->readerMutex.unlock();
        return message;
    }

    bool ConnectedClient::HasMessage() {
        return !this->readQueue.empty();
    }

    void ConnectedClient::ReadLoop() {

        int bytesReceived = 0;
        std::array<char, 4096> receiveBuffer{};

        while (this->isActive) {
            bytesReceived = recv(this->socket, &receiveBuffer[0], READ_BUFFER_SIZE, 0);

            if (bytesReceived > 0)  {
                auto lock = std::lock_guard(this->readerMutex);

                // TODO delimiter
                this->readQueue.emplace(receiveBuffer.cbegin(), receiveBuffer.cbegin() + bytesReceived);
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

