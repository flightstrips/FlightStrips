#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "messages/MessageService.h"

// MessageService constructor requires shared_ptr<FlightStripsPlugin> and
// several other heavyweight services that depend on Win32/EuroScope
// infrastructure unavailable in unit tests.
//
// We therefore only compile-test the public interface (header inclusion,
// type availability) and document what would be covered with proper seams.
//
// If MessageService were refactored to accept lightweight interfaces, the
// following behaviours should be tested per message type:
//
//  - "assigned_squawk"      → GetFlightPlan; SetSquawk
//  - "ground_state"         → SetGroundState
//  - "stand"                → SetStand
//  - "sid"                  → SetSid via RouteService
//  - "aircraft_runway"      → SetRunway via RouteService
//  - "session_info"         → stores session info
//  - ... (all HandleXxxEvent helpers)

TEST(MessageServiceTest, HeaderCompiles) {
    using FlightStrips::messages::MessageService;
    SUCCEED();
}
