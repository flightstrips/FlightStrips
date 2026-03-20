#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "handlers/ConnectionEventHandlers.h"
#include "mock/MockConnectionEventHandler.h"

using FlightStrips::handlers::ConnectionEventHandlers;
using ::testing::StrictMock;

class ConnectionEventHandlersTest : public ::testing::Test {
protected:
    ConnectionEventHandlers handlers;
};

TEST_F(ConnectionEventHandlersTest, NoHandlers_DoesNotCrash) {
    EXPECT_NO_FATAL_FAILURE(handlers.OnOnline());
}

TEST_F(ConnectionEventHandlersTest, OnOnline_CallsAllHandlers) {
    auto h1 = std::make_shared<StrictMock<MockConnectionEventHandler>>();
    auto h2 = std::make_shared<StrictMock<MockConnectionEventHandler>>();

    EXPECT_CALL(*h1, Online()).Times(1);
    EXPECT_CALL(*h2, Online()).Times(1);

    handlers.RegisterHandler(h1);
    handlers.RegisterHandler(h2);
    handlers.OnOnline();
}

TEST_F(ConnectionEventHandlersTest, Clear_RemovesAllHandlers) {
    auto h = std::make_shared<StrictMock<MockConnectionEventHandler>>();
    handlers.RegisterHandler(h);
    handlers.Clear();
    // No calls expected.
    handlers.OnOnline();
}

TEST_F(ConnectionEventHandlersTest, RegisterSameHandlerTwice_CallsItTwice) {
    auto h = std::make_shared<StrictMock<MockConnectionEventHandler>>();
    EXPECT_CALL(*h, Online()).Times(2);

    handlers.RegisterHandler(h);
    handlers.RegisterHandler(h);
    handlers.OnOnline();
}
