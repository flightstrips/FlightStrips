#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "controller/ControllerService.h"

// ControllerService constructor requires a shared_ptr<WebSocketService>,
// which is a heavyweight object that depends on ASIO and Win32/EuroScope
// infrastructure unavailable in unit tests.
//
// We therefore only compile-test the public interface (header inclusion,
// type availability) and document what would be covered with proper seams.
//
// If ControllerService accepted an IWebSocketClient interface, the following
// behaviours should be tested:
//
//  - ControllerPositionUpdateEvent(ctrl) → adds/updates controller entry
//    and calls SendEvent<ControllerOnlineEvent>
//  - ControllerDisconnectEvent(ctrl)    → removes controller and calls
//    SendEvent<ControllerOfflineEvent>

TEST(ControllerServiceTest, HeaderCompiles) {
    using FlightStrips::controller::ControllerService;
    SUCCEED();
}
