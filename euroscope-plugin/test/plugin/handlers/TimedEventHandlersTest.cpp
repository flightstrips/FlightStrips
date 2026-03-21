#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "handlers/TimedEventHandlers.h"
#include "mock/MockTimedEventHandler.h"

using FlightStrips::handlers::TimedEventHandlers;
using ::testing::StrictMock;

class TimedEventHandlersTest : public ::testing::Test {
protected:
    TimedEventHandlers handlers;
};

TEST_F(TimedEventHandlersTest, NoHandlers_DoesNotCrash) {
    EXPECT_NO_FATAL_FAILURE(handlers.OnTimer(0));
}

TEST_F(TimedEventHandlersTest, OnTimer_CallsAllHandlers) {
    auto h1 = std::make_shared<StrictMock<MockTimedEventHandler>>();
    auto h2 = std::make_shared<StrictMock<MockTimedEventHandler>>();

    EXPECT_CALL(*h1, OnTimer(60)).Times(1);
    EXPECT_CALL(*h2, OnTimer(60)).Times(1);

    handlers.RegisterHandler(h1);
    handlers.RegisterHandler(h2);
    handlers.OnTimer(60);
}

TEST_F(TimedEventHandlersTest, OnTimer_PassesCorrectTime) {
    auto h = std::make_shared<StrictMock<MockTimedEventHandler>>();
    EXPECT_CALL(*h, OnTimer(1234)).Times(1);

    handlers.RegisterHandler(h);
    handlers.OnTimer(1234);
}

TEST_F(TimedEventHandlersTest, Clear_RemovesAllHandlers) {
    auto h = std::make_shared<StrictMock<MockTimedEventHandler>>();
    handlers.RegisterHandler(h);
    handlers.Clear();
    // No calls expected — StrictMock would fail if any occurred.
    handlers.OnTimer(99);
}

TEST_F(TimedEventHandlersTest, RegisterSameHandlerTwice_CallsItTwice) {
    auto h = std::make_shared<StrictMock<MockTimedEventHandler>>();
    EXPECT_CALL(*h, OnTimer(5)).Times(2);

    handlers.RegisterHandler(h);
    handlers.RegisterHandler(h);
    handlers.OnTimer(5);
}
