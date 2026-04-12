#include <gtest/gtest.h>

#include "tag_items/ClearanceStatusHandler.h"

using FlightStrips::TagItems::ClearanceStatusHandler;
using FlightStrips::flightplan::FlightPlan;

TEST(ClearanceStatusHandlerTest, ResolvePresentation_ClearedStateShowsSent) {
    const auto presentation = ClearanceStatusHandler::ResolvePresentation(
        FlightPlan{.pdc_state = "CLEARED"},
        false
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "SENT");
    EXPECT_EQ(presentation.color, RGB(110, 153, 110));
}

TEST(ClearanceStatusHandlerTest, ResolvePresentation_ConfirmedStateShowsDoneWithoutEsClearFlag) {
    const auto presentation = ClearanceStatusHandler::ResolvePresentation(
        FlightPlan{.pdc_state = "CONFIRMED"},
        false
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "DONE");
    EXPECT_EQ(presentation.color, RGB(110, 153, 110));
}

TEST(ClearanceStatusHandlerTest, ResolvePresentation_EsClearFlagStillShowsOk) {
    const auto presentation = ClearanceStatusHandler::ResolvePresentation(
        FlightPlan{.pdc_state = "CLEARED"},
        true
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "OK");
    EXPECT_EQ(presentation.color, RGB(110, 153, 110));
}

TEST(ClearanceStatusHandlerTest, ResolvePresentation_RevertToVoiceShowsRt) {
    const auto presentation = ClearanceStatusHandler::ResolvePresentation(
        FlightPlan{.pdc_state = "REVERT_TO_VOICE"},
        false
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "R/T");
    EXPECT_EQ(presentation.color, RGB(110, 153, 110));
}

TEST(ClearanceStatusHandlerTest, ResolvePresentation_RequestedWithRemarksShowsReqDot) {
    const auto presentation = ClearanceStatusHandler::ResolvePresentation(
        FlightPlan{.pdc_state = "REQUESTED", .pdc_request_remarks = "NO SID"},
        false
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "REQ*");
    EXPECT_EQ(presentation.color, RGB(110, 153, 110));
}

TEST(ClearanceStatusHandlerTest, ResolvePresentation_RequestedWithFaultsAndRemarksShowsYellowReqDot) {
    const auto presentation = ClearanceStatusHandler::ResolvePresentation(
        FlightPlan{.pdc_state = "REQUESTED_WITH_FAULTS", .pdc_request_remarks = "NO SID"},
        false
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "REQ*");
    EXPECT_EQ(presentation.color, RGB(212, 214, 7));
}
