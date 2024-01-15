//
// Created by fsr19 on 25/05/2023.
//

#pragma once


#include <semaphore>
#include "MessageHandler.h"

namespace FlightStrips {
    struct Container;
    namespace network {
        class MessageHandler;
    }
}

namespace FlightStrips::network {
    class ConnectedClient {
    public:
        ConnectedClient(SOCKET socket, const std::shared_ptr<Container>& mContainer);
        ~ConnectedClient();

        void Write(const std::string &message);

        bool IsActive() const;
    private:

        void WriteLoop();

        void ReadLoop();

        SOCKET socket;
        bool isActive;

        std::queue<std::string> writeQueue{};

        std::unique_ptr<std::thread> writerThread;
        std::unique_ptr<std::thread> readerThread;

        std::counting_semaphore<100> writerSemaphore{0};
        std::mutex writerMutex;

        MessageHandler m_messageHandler;

        static inline const int READ_BUFFER_SIZE = 4096;
    };
}
