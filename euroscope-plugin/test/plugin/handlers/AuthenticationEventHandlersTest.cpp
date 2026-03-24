#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "handlers/AuthenticationEventHandlers.h"
#include "mock/MockAuthenticationEventHandler.h"

using FlightStrips::handlers::AuthenticationEventHandlers;
using ::testing::StrictMock;

class AuthenticationEventHandlersTest : public ::testing::Test {
protected:
    AuthenticationEventHandlers handlers;
};

TEST_F(AuthenticationEventHandlersTest, NoHandlers_DoesNotCrash) {
    EXPECT_NO_FATAL_FAILURE(handlers.OnTokenUpdate("some-token"));
}

TEST_F(AuthenticationEventHandlersTest, OnTokenUpdate_CallsAllHandlers) {
    auto h1 = std::make_shared<StrictMock<MockAuthenticationEventHandler>>();
    auto h2 = std::make_shared<StrictMock<MockAuthenticationEventHandler>>();

    EXPECT_CALL(*h1, OnTokenUpdate("my-token")).Times(1);
    EXPECT_CALL(*h2, OnTokenUpdate("my-token")).Times(1);

    handlers.RegisterHandler(h1);
    handlers.RegisterHandler(h2);
    handlers.OnTokenUpdate("my-token");
}

TEST_F(AuthenticationEventHandlersTest, OnTokenUpdate_PassesCorrectToken) {
    auto h = std::make_shared<StrictMock<MockAuthenticationEventHandler>>();
    EXPECT_CALL(*h, OnTokenUpdate("abc123")).Times(1);

    handlers.RegisterHandler(h);
    handlers.OnTokenUpdate("abc123");
}

TEST_F(AuthenticationEventHandlersTest, Clear_RemovesAllHandlers) {
    auto h = std::make_shared<StrictMock<MockAuthenticationEventHandler>>();
    handlers.RegisterHandler(h);
    handlers.Clear();
    // No calls expected.
    handlers.OnTokenUpdate("token");
}

TEST_F(AuthenticationEventHandlersTest, RegisterSameHandlerTwice_CallsItTwice) {
    auto h = std::make_shared<StrictMock<MockAuthenticationEventHandler>>();
    EXPECT_CALL(*h, OnTokenUpdate("t")).Times(2);

    handlers.RegisterHandler(h);
    handlers.RegisterHandler(h);
    handlers.OnTokenUpdate("t");
}
