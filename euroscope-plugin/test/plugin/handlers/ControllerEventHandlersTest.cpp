#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "handlers/ControllerEventHandlers.h"
#include "mock/MockControllerEventHandler.h"

using FlightStrips::handlers::ControllerEventHandlers;
using ::testing::StrictMock;

static EuroScopePlugIn::CController MakeCtrl() {
    return EuroScopePlugIn::CController();
}

class ControllerEventHandlersTest : public ::testing::Test {
protected:
    ControllerEventHandlers handlers;
};

TEST_F(ControllerEventHandlersTest, NoHandlers_DoesNotCrash) {
    EXPECT_NO_FATAL_FAILURE(handlers.ControllerPositionUpdateEvent(MakeCtrl()));
    EXPECT_NO_FATAL_FAILURE(handlers.ControllerDisconnectEvent(MakeCtrl()));
}

TEST_F(ControllerEventHandlersTest, ControllerPositionUpdateEvent_CallsAllHandlers) {
    auto h1 = std::make_shared<StrictMock<MockControllerEventHandler>>();
    auto h2 = std::make_shared<StrictMock<MockControllerEventHandler>>();

    EXPECT_CALL(*h1, ControllerPositionUpdateEvent(::testing::_)).Times(1);
    EXPECT_CALL(*h2, ControllerPositionUpdateEvent(::testing::_)).Times(1);

    handlers.RegisterHandler(h1);
    handlers.RegisterHandler(h2);
    handlers.ControllerPositionUpdateEvent(MakeCtrl());
}

TEST_F(ControllerEventHandlersTest, ControllerDisconnectEvent_CallsAllHandlers) {
    auto h1 = std::make_shared<StrictMock<MockControllerEventHandler>>();
    auto h2 = std::make_shared<StrictMock<MockControllerEventHandler>>();

    EXPECT_CALL(*h1, ControllerDisconnectEvent(::testing::_)).Times(1);
    EXPECT_CALL(*h2, ControllerDisconnectEvent(::testing::_)).Times(1);

    handlers.RegisterHandler(h1);
    handlers.RegisterHandler(h2);
    handlers.ControllerDisconnectEvent(MakeCtrl());
}

TEST_F(ControllerEventHandlersTest, Clear_RemovesAllHandlers) {
    auto h = std::make_shared<StrictMock<MockControllerEventHandler>>();
    handlers.RegisterHandler(h);
    handlers.Clear();
    handlers.ControllerPositionUpdateEvent(MakeCtrl());
    handlers.ControllerDisconnectEvent(MakeCtrl());
}

TEST_F(ControllerEventHandlersTest, RegisterSameHandlerTwice_CallsItTwice) {
    auto h = std::make_shared<StrictMock<MockControllerEventHandler>>();
    EXPECT_CALL(*h, ControllerPositionUpdateEvent(::testing::_)).Times(2);

    handlers.RegisterHandler(h);
    handlers.RegisterHandler(h);
    handlers.ControllerPositionUpdateEvent(MakeCtrl());
}
