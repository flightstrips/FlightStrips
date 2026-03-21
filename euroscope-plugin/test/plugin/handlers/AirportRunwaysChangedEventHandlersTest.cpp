#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "handlers/AirportRunwaysChangedEventHandlers.h"
#include "mock/MockAirportRunwaysChangedEventHandler.h"

using FlightStrips::handlers::AirportRunwaysChangedEventHandlers;
using ::testing::StrictMock;

class AirportRunwaysChangedEventHandlersTest : public ::testing::Test {
protected:
    AirportRunwaysChangedEventHandlers handlers;
};

TEST_F(AirportRunwaysChangedEventHandlersTest, NoHandlers_DoesNotCrash) {
    EXPECT_NO_FATAL_FAILURE(handlers.OnAirportRunwayActivityChanged());
}

TEST_F(AirportRunwaysChangedEventHandlersTest, OnAirportRunwayActivityChanged_CallsAllHandlers) {
    auto h1 = std::make_shared<StrictMock<MockAirportRunwaysChangedEventHandler>>();
    auto h2 = std::make_shared<StrictMock<MockAirportRunwaysChangedEventHandler>>();

    EXPECT_CALL(*h1, OnAirportRunwayActivityChanged()).Times(1);
    EXPECT_CALL(*h2, OnAirportRunwayActivityChanged()).Times(1);

    handlers.RegisterHandler(h1);
    handlers.RegisterHandler(h2);
    handlers.OnAirportRunwayActivityChanged();
}

TEST_F(AirportRunwaysChangedEventHandlersTest, Clear_RemovesAllHandlers) {
    auto h = std::make_shared<StrictMock<MockAirportRunwaysChangedEventHandler>>();
    handlers.RegisterHandler(h);
    handlers.Clear();
    // No calls expected.
    handlers.OnAirportRunwayActivityChanged();
}

TEST_F(AirportRunwaysChangedEventHandlersTest, RegisterSameHandlerTwice_CallsItTwice) {
    auto h = std::make_shared<StrictMock<MockAirportRunwaysChangedEventHandler>>();
    EXPECT_CALL(*h, OnAirportRunwayActivityChanged()).Times(2);

    handlers.RegisterHandler(h);
    handlers.RegisterHandler(h);
    handlers.OnAirportRunwayActivityChanged();
}
