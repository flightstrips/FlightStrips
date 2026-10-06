#include <gtest/gtest.h>
#include "tag_items/SharedFieldHandler.h"

using FlightStrips::TagItems::SharedFieldHandler;
using FlightStrips::flightplan::FlightPlan;

TEST(SharedFieldHandlerTest, HoldingReleaseUsesAuthoritativeEatOnNonTrackingClients) {
    FlightPlan plan;
    plan.hold = "MONAK";
    plan.hold_eat = "1200";
    EXPECT_EQ(SharedFieldHandler::Resolve(&plan, SharedFieldHandler::Field::Eat), "1200");
    plan.backend_hold_eat_replay = FlightStrips::flightplan::BackendHoldEatReplay{"MONAK", "enroute", "1215"};
    EXPECT_EQ(SharedFieldHandler::Resolve(&plan, SharedFieldHandler::Field::Eat), "1215");
    FlightStrips::flightplan::ApplyTopSkyHoldCommand(plan, {FlightStrips::flightplan::TopSkyHoldCommandType::Cancel});
    EXPECT_EQ(SharedFieldHandler::Resolve(&plan, SharedFieldHandler::Field::Eat), "");
}

TEST(SharedFieldHandlerTest, SharedScratchPadDoesNotUseEuroScopeScratchCommands) {
    FlightPlan plan;
    plan.fs_scratch_pad = "CALL OPS";
    EXPECT_EQ(SharedFieldHandler::Resolve(&plan, SharedFieldHandler::Field::ScratchPad), "CALL OPS");
    plan.fs_scratch_pad.clear();
    EXPECT_EQ(SharedFieldHandler::Resolve(&plan, SharedFieldHandler::Field::ScratchPad), "");
    EXPECT_EQ(SharedFieldHandler::Resolve(nullptr, SharedFieldHandler::Field::ScratchPad), "");
}

TEST(SharedFieldHandlerTest, ServerClearHidesStaleObservedEat) {
    FlightStrips::flightplan::FlightPlanService service(nullptr, nullptr, nullptr, nullptr, nullptr);
    service.ApplyBackendSyncHold("SAS123", "MONAK", "enroute", "1200");
    service.CacheBackendHoldEatReplay("SAS123", "MONAK", "enroute", "1215");
    EXPECT_EQ(SharedFieldHandler::Resolve(service.GetFlightPlan("SAS123"), SharedFieldHandler::Field::Eat), "1215");
    service.CacheBackendHoldEatReplay("SAS123", "", "", "");
    EXPECT_EQ(SharedFieldHandler::Resolve(service.GetFlightPlan("SAS123"), SharedFieldHandler::Field::Eat), "");
}
