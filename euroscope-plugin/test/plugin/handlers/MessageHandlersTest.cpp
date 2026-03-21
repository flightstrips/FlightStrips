#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "handlers/MessageHandlers.h"
#include "mock/MockMessageHandler.h"

using FlightStrips::handlers::MessageHandlers;
using ::testing::StrictMock;
using ::testing::ElementsAre;

class MessageHandlersTest : public ::testing::Test {
protected:
    MessageHandlers handlers;
};

TEST_F(MessageHandlersTest, NoHandlers_DoesNotCrash) {
    EXPECT_NO_FATAL_FAILURE(handlers.OnMessages({}));
}

TEST_F(MessageHandlersTest, OnMessages_CallsAllHandlers) {
    auto h1 = std::make_shared<StrictMock<MockMessageHandler>>();
    auto h2 = std::make_shared<StrictMock<MockMessageHandler>>();

    std::vector<nlohmann::json> msgs = {nlohmann::json{{"type", "test"}}};

    EXPECT_CALL(*h1, OnMessages(msgs)).Times(1);
    EXPECT_CALL(*h2, OnMessages(msgs)).Times(1);

    handlers.RegisterHandler(h1);
    handlers.RegisterHandler(h2);
    handlers.OnMessages(msgs);
}

TEST_F(MessageHandlersTest, OnMessages_EmptyList_CallsHandlers) {
    auto h = std::make_shared<StrictMock<MockMessageHandler>>();
    std::vector<nlohmann::json> empty;
    EXPECT_CALL(*h, OnMessages(empty)).Times(1);

    handlers.RegisterHandler(h);
    handlers.OnMessages(empty);
}

TEST_F(MessageHandlersTest, Clear_RemovesAllHandlers) {
    auto h = std::make_shared<StrictMock<MockMessageHandler>>();
    handlers.RegisterHandler(h);
    handlers.Clear();
    // No calls expected.
    handlers.OnMessages({});
}

TEST_F(MessageHandlersTest, RegisterSameHandlerTwice_CallsItTwice) {
    auto h = std::make_shared<StrictMock<MockMessageHandler>>();
    std::vector<nlohmann::json> msgs;
    EXPECT_CALL(*h, OnMessages(msgs)).Times(2);

    handlers.RegisterHandler(h);
    handlers.RegisterHandler(h);
    handlers.OnMessages(msgs);
}
