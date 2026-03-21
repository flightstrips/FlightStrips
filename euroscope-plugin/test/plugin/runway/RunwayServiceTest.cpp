#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "runway/RunwayService.h"

// RunwayService constructor requires a shared_ptr<WebSocketService> and a
// shared_ptr<FlightStripsPlugin>, both of which are heavyweight objects
// that depend on Win32/EuroScope infrastructure unavailable in unit tests.
//
// We therefore only compile-test the public interface (header inclusion,
// type availability) and document what would be covered with proper seams.
//
// If RunwayService were refactored to accept interfaces rather than
// concrete types, the following behaviours should be tested:
//
//  - Online()   → calls WebSocketService::SendEvent<RunwayEvent>
//  - OnAirportRunwayActivityChanged() → calls WebSocketService::SendEvent<RunwayEvent>
//  - GetActiveRunways() → returns runways reported by the plugin for the airport

TEST(RunwayServiceTest, HeaderCompiles) {
    // This test verifies that the RunwayService header is self-contained and
    // that the type is accessible from test code.
    using FlightStrips::runway::RunwayService;
    SUCCEED();
}
