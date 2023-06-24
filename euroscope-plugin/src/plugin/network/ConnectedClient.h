//
// Created by fsr19 on 25/05/2023.
//

#pragma once


#include <semaphore>

namespace FlightStrips::network {
    class ConnectedClient {
    public:
        explicit ConnectedClient(SOCKET socket);
        ~ConnectedClient();

        void Write(const std::string &message);

        bool HasMessage();
        std::string Read();
        bool IsActive() const;
    private:

        void WriteLoop();

        void ReadLoop();

        SOCKET socket;
        bool isActive;

        std::queue<std::string> writeQueue{};
        std::queue<std::string> readQueue{};

        std::unique_ptr<std::thread> writerThread;
        std::unique_ptr<std::thread> readerThread;

        std::counting_semaphore<100> writerSemaphore{0};
        std::mutex writerMutex;
        std::mutex readerMutex;

        static inline const int READ_BUFFER_SIZE = 4096;
    };
}
