#include <gtest/gtest.h>

#include "handlers/GroundTagReleaseHandler.h"

using FlightStrips::handlers::GroundTagReleaseHandler;

TEST(GroundTagReleaseHandlerTest, IgnoresNonScratchpadControllerData) {
    GroundTagReleaseHandler handler({});

    EXPECT_NO_FATAL_FAILURE(handler.ControllerFlightPlanDataEvent(
        EuroScopePlugIn::CFlightPlan(), EuroScopePlugIn::CTR_DATA_TYPE_HEADING));
}

TEST(GroundTagReleaseHandlerTest, IgnoresInvalidFlightPlanWithoutPlugin) {
    GroundTagReleaseHandler handler({});

    EXPECT_NO_FATAL_FAILURE(handler.ControllerFlightPlanDataEvent(
        EuroScopePlugIn::CFlightPlan(), EuroScopePlugIn::CTR_DATA_TYPE_SCRATCH_PAD_STRING));
}

TEST(GroundTagReleaseHandlerTest, IgnoresFlightPlanUpdatesAndDisconnects) {
    GroundTagReleaseHandler handler({});

    EXPECT_NO_FATAL_FAILURE(handler.FlightPlanEvent(EuroScopePlugIn::CFlightPlan()));
    EXPECT_NO_FATAL_FAILURE(handler.FlightPlanDisconnectEvent(EuroScopePlugIn::CFlightPlan()));
}
