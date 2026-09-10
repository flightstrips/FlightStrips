#pragma once
#include <string>
#include <vector>

namespace FlightStrips::handlers {
    class MessageHandler {
    public:
        virtual ~MessageHandler() = default;
        virtual void OnMessages(const std::vector<std::string>& messages) = 0;
    };
}
