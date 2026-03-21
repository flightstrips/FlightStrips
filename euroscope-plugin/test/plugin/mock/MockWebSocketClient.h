#pragma once
#include <gmock/gmock.h>
#include "websocket/IWebSocketClient.h"

class MockWebSocketClient : public FlightStrips::websocket::IWebSocketClient {
public:
    MOCK_METHOD1(Send, void(const nlohmann::json& message));
    MOCK_CONST_METHOD0(IsConnected, bool());
};
