#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "handlers/FlightPlanEventHandlers.h"
#include "mock/MockFlightPlanEventHandler.h"

using FlightStrips::handlers::FlightPlanEventHandlers;
using ::testing::StrictMock;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/// Build a default-constructed CFlightPlan (all fields zeroed by EuroScope).
static EuroScopePlugIn::CFlightPlan MakeFP() {
    return EuroScopePlugIn::CFlightPlan();
}

// ---------------------------------------------------------------------------
// FlightPlanEventHandlers — basic dispatch
// ---------------------------------------------------------------------------

class FlightPlanEventHandlersTest : public ::testing::Test {
protected:
    FlightPlanEventHandlers handlers;
};

TEST_F(FlightPlanEventHandlersTest, RegisterHandler_NullDoesNotCrash) {
    // Registering nothing and calling events should not crash.
    EXPECT_NO_FATAL_FAILURE(handlers.FlightPlanEvent(MakeFP()));
    EXPECT_NO_FATAL_FAILURE(handlers.ControllerFlightPlanDataEvent(MakeFP(), 0));
    EXPECT_NO_FATAL_FAILURE(handlers.FlightPlanDisconnectEvent(MakeFP()));
}

TEST_F(FlightPlanEventHandlersTest, FlightPlanEvent_CallsAllRegisteredHandlers) {
    auto h1 = std::make_shared<StrictMock<MockFlightPlanEventHandler>>();
    auto h2 = std::make_shared<StrictMock<MockFlightPlanEventHandler>>();

    EXPECT_CALL(*h1, FlightPlanEvent(::testing::_)).Times(1);
    EXPECT_CALL(*h2, FlightPlanEvent(::testing::_)).Times(1);

    handlers.RegisterHandler(h1);
    handlers.RegisterHandler(h2);
    handlers.FlightPlanEvent(MakeFP());
}

TEST_F(FlightPlanEventHandlersTest, ControllerFlightPlanDataEvent_CallsAllRegisteredHandlers) {
    auto h1 = std::make_shared<StrictMock<MockFlightPlanEventHandler>>();
    auto h2 = std::make_shared<StrictMock<MockFlightPlanEventHandler>>();

    EXPECT_CALL(*h1, ControllerFlightPlanDataEvent(::testing::_, 42)).Times(1);
    EXPECT_CALL(*h2, ControllerFlightPlanDataEvent(::testing::_, 42)).Times(1);

    handlers.RegisterHandler(h1);
    handlers.RegisterHandler(h2);
    handlers.ControllerFlightPlanDataEvent(MakeFP(), 42);
}

TEST_F(FlightPlanEventHandlersTest, FlightPlanDisconnectEvent_CallsAllRegisteredHandlers) {
    auto h1 = std::make_shared<StrictMock<MockFlightPlanEventHandler>>();
    auto h2 = std::make_shared<StrictMock<MockFlightPlanEventHandler>>();

    EXPECT_CALL(*h1, FlightPlanDisconnectEvent(::testing::_)).Times(1);
    EXPECT_CALL(*h2, FlightPlanDisconnectEvent(::testing::_)).Times(1);

    handlers.RegisterHandler(h1);
    handlers.RegisterHandler(h2);
    handlers.FlightPlanDisconnectEvent(MakeFP());
}

TEST_F(FlightPlanEventHandlersTest, Clear_RemovesAllHandlers) {
    auto h = std::make_shared<StrictMock<MockFlightPlanEventHandler>>();
    // After Clear(), no calls expected — StrictMock will fail if any occur.
    handlers.RegisterHandler(h);
    handlers.Clear();
    handlers.FlightPlanEvent(MakeFP());
    handlers.ControllerFlightPlanDataEvent(MakeFP(), 0);
    handlers.FlightPlanDisconnectEvent(MakeFP());
}

TEST_F(FlightPlanEventHandlersTest, RegisterSameHandlerTwice_CallsItTwice) {
    // Current implementation push_backs without dedup.
    auto h = std::make_shared<StrictMock<MockFlightPlanEventHandler>>();
    EXPECT_CALL(*h, FlightPlanEvent(::testing::_)).Times(2);

    handlers.RegisterHandler(h);
    handlers.RegisterHandler(h);
    handlers.FlightPlanEvent(MakeFP());
}
