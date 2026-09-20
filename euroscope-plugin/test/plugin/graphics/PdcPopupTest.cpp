#include <gtest/gtest.h>

#include "graphics/PdcPopup.h"

using FlightStrips::graphics::HasRequestRemarks;
using FlightStrips::graphics::IsRequestedPdcState;
using FlightStrips::graphics::CalculatePdcPopupSize;
using FlightStrips::graphics::PdcPopupData;
using FlightStrips::graphics::PdcPopupPrimaryAction;
using FlightStrips::graphics::ResolvePdcPopupPrimaryAction;
using FlightStrips::graphics::ShouldSendPdcRevertToVoice;

TEST(PdcPopupTest, HasRequestRemarksIgnoresBlankValues) {
    EXPECT_FALSE(HasRequestRemarks(""));
    EXPECT_FALSE(HasRequestRemarks("   "));
    EXPECT_TRUE(HasRequestRemarks("NO SID"));
}

TEST(PdcPopupTest, IsRequestedPdcStateRecognizesActiveRequestStates) {
    EXPECT_TRUE(IsRequestedPdcState("REQUESTED"));
    EXPECT_TRUE(IsRequestedPdcState("REQUESTED_WITH_FAULTS"));
    EXPECT_FALSE(IsRequestedPdcState("CLEARED"));
}

TEST(PdcPopupTest, ResolvePrimaryActionDoesNothingWhenAlreadyCleared) {
    EXPECT_EQ(ResolvePdcPopupPrimaryAction("REQUESTED", true), PdcPopupPrimaryAction::None);
}

TEST(PdcPopupTest, ResolvePrimaryActionIssuesClearanceForRequestedState) {
    EXPECT_EQ(ResolvePdcPopupPrimaryAction("REQUESTED", false),
              PdcPopupPrimaryAction::IssueRequestedClearance);
}

TEST(PdcPopupTest, ResolvePrimaryActionIssuesClearanceForRequestedStateWithFaults) {
    EXPECT_EQ(ResolvePdcPopupPrimaryAction("REQUESTED_WITH_FAULTS", false),
              PdcPopupPrimaryAction::IssueRequestedClearance);
}

TEST(PdcPopupTest, ResolvePrimaryActionFallsBackToEuroScopeClearanceForNonRequestedStates) {
    EXPECT_EQ(ResolvePdcPopupPrimaryAction("CONFIRMED", false),
              PdcPopupPrimaryAction::SetEuroscopeClearance);
}

TEST(PdcPopupTest, RevertToVoiceIsAllowedForBothRequestedStates) {
    EXPECT_TRUE(ShouldSendPdcRevertToVoice("REQUESTED"));
    EXPECT_TRUE(ShouldSendPdcRevertToVoice("REQUESTED_WITH_FAULTS"));
    EXPECT_FALSE(ShouldSendPdcRevertToVoice("CLEARED"));
}

TEST(PdcPopupTest, CalculatePopupSizeAccountsForRequestedClearanceRemarks) {
    PdcPopupData data{.requestRemarks = "NO SID", .pdcState = "REQUESTED"};

    const auto withRemarks = CalculatePdcPopupSize(data);
    data.requestRemarks.clear();
    const auto withoutRemarks = CalculatePdcPopupSize(data);

    EXPECT_EQ(withRemarks.width, 265);
    EXPECT_EQ(withRemarks.height, 233);
    EXPECT_EQ(withoutRemarks.width, 265);
    EXPECT_EQ(withoutRemarks.height, 215);
}

TEST(PdcPopupTest, CalculatePopupSizeUsesCompactSizeForNonRequestState) {
    const PdcPopupData data{.pdcState = "CLEARED"};

    const auto size = CalculatePdcPopupSize(data);

    EXPECT_EQ(size.width, 132);
    EXPECT_EQ(size.height, 165);
}
