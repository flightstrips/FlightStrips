#pragma once

#include <string>

namespace FlightStrips::messages {
    class PrivateMessageSender {
    public:
        static void SendPrivateMessage(const std::string& callsign, const std::string& message);

    private:
        static void TypeString(const std::string& text);
        static void PressKey(unsigned short vk, bool keyDown);
        static void PressEnter();
    };
}