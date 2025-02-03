#pragma once
#include <nlohmann/json.hpp>

namespace FlightStrips::handlers {
    class MessageHandler {
    public:
        virtual ~MessageHandler() = default;
        virtual void OnMessages(const std::vector<nlohmann::json>& messages) = 0;
    };
}
