#pragma once

#include <string>

namespace FlightStrips::messages {
    class PrivateMessageSender {
    public:
        static void SendPrivateMessage(const std::string& callsign, const std::string& message);

    private:
        static bool SendCommandViaMessageInput(const std::string& command);
    };
}
