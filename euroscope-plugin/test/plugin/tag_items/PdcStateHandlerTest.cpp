#include <gtest/gtest.h>

#include "tag_items/PdcStateHandler.h"

using FlightStrips::TagItems::PdcStateHandler;
using FlightStrips::flightplan::FlightPlan;

TEST(PdcStateHandlerTest, ResolvePresentation_ClearedStateShowsSent) {
    const auto presentation = PdcStateHandler::ResolvePresentation(
        FlightPlan{.pdc_state = "CLEARED"},
        false
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "SENT");
    EXPECT_EQ(presentation.color, RGB(110, 153, 110));
}

TEST(PdcStateHandlerTest, ResolvePresentation_ConfirmedStateShowsDoneWithoutEsClearFlag) {
    const auto presentation = PdcStateHandler::ResolvePresentation(
        FlightPlan{.pdc_state = "CONFIRMED"},
        false
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "DONE");
    EXPECT_EQ(presentation.color, RGB(110, 153, 110));
}

TEST(PdcStateHandlerTest, ResolvePresentation_EsClearFlagStillShowsOk) {
    const auto presentation = PdcStateHandler::ResolvePresentation(
        FlightPlan{.pdc_state = "CLEARED"},
        true
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "OK");
    EXPECT_EQ(presentation.color, RGB(110, 153, 110));
}

TEST(PdcStateHandlerTest, ResolvePresentation_RevertToVoiceShowsRt) {
    const auto presentation = PdcStateHandler::ResolvePresentation(
        FlightPlan{.pdc_state = "REVERT_TO_VOICE"},
        false
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "R/T");
    EXPECT_EQ(presentation.color, RGB(110, 153, 110));
}
