#pragma once
#include <nlohmann/json.hpp>

namespace FlightStrips::websocket {

/// Pure-virtual interface for WebSocket send operations.
/// The production WebSocket class implements this; tests use MockWebSocketClient.
class IWebSocketClient {
public:
    virtual ~IWebSocketClient() = default;

    virtual void Send(const nlohmann::json& message) = 0;
    virtual bool IsConnected() const = 0;
};

} // namespace FlightStrips::websocket
