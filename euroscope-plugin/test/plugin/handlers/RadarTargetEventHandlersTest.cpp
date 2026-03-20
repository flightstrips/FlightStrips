#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "handlers/RadarTargetEventHandlers.h"
#include "mock/MockRadarTargetEventHandler.h"

using FlightStrips::handlers::RadarTargetEventHandlers;
using ::testing::StrictMock;

static EuroScopePlugIn::CRadarTarget MakeRT() {
    return EuroScopePlugIn::CRadarTarget();
}

class RadarTargetEventHandlersTest : public ::testing::Test {
protected:
    RadarTargetEventHandlers handlers;
};

TEST_F(RadarTargetEventHandlersTest, NoHandlers_DoesNotCrash) {
    EXPECT_NO_FATAL_FAILURE(handlers.RadarTargetPositionEvent(MakeRT(), false));
    EXPECT_NO_FATAL_FAILURE(handlers.RadarTargetOutOfRangeEvent(MakeRT()));
}

TEST_F(RadarTargetEventHandlersTest, RadarTargetPositionEvent_CallsAllHandlers) {
    auto h1 = std::make_shared<StrictMock<MockRadarTargetEventHandler>>();
    auto h2 = std::make_shared<StrictMock<MockRadarTargetEventHandler>>();

    EXPECT_CALL(*h1, RadarTargetPositionEvent(::testing::_, true)).Times(1);
    EXPECT_CALL(*h2, RadarTargetPositionEvent(::testing::_, true)).Times(1);

    handlers.RegisterHandler(h1);
    handlers.RegisterHandler(h2);
    handlers.RadarTargetPositionEvent(MakeRT(), true);
}

TEST_F(RadarTargetEventHandlersTest, RadarTargetOutOfRangeEvent_CallsAllHandlers) {
    auto h1 = std::make_shared<StrictMock<MockRadarTargetEventHandler>>();
    auto h2 = std::make_shared<StrictMock<MockRadarTargetEventHandler>>();

    EXPECT_CALL(*h1, RadarTargetOutOfRangeEvent(::testing::_)).Times(1);
    EXPECT_CALL(*h2, RadarTargetOutOfRangeEvent(::testing::_)).Times(1);

    handlers.RegisterHandler(h1);
    handlers.RegisterHandler(h2);
    handlers.RadarTargetOutOfRangeEvent(MakeRT());
}

TEST_F(RadarTargetEventHandlersTest, Clear_RemovesAllHandlers) {
    auto h = std::make_shared<StrictMock<MockRadarTargetEventHandler>>();
    handlers.RegisterHandler(h);
    handlers.Clear();
    // StrictMock: no calls expected — any call would fail the test.
    handlers.RadarTargetPositionEvent(MakeRT(), false);
    handlers.RadarTargetOutOfRangeEvent(MakeRT());
}

TEST_F(RadarTargetEventHandlersTest, RegisterSameHandlerTwice_CallsItTwice) {
    auto h = std::make_shared<StrictMock<MockRadarTargetEventHandler>>();
    EXPECT_CALL(*h, RadarTargetPositionEvent(::testing::_, false)).Times(2);

    handlers.RegisterHandler(h);
    handlers.RegisterHandler(h);
    handlers.RadarTargetPositionEvent(MakeRT(), false);
}
