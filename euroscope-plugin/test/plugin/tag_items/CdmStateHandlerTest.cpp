#include <gtest/gtest.h>

#include <chrono>
#include <ctime>
#include <format>

#include "tag_items/CdmStateHandler.h"

using FlightStrips::TagItems::CdmStateHandler;
using FlightStrips::flightplan::CdmState;

namespace {
    auto CurrentUtcHHMM(int deltaMinutes = 0) -> std::string {
        const auto now = std::chrono::system_clock::now() + std::chrono::minutes(deltaMinutes);
        const auto nowTime = std::chrono::system_clock::to_time_t(now);
        std::tm utc{};
        gmtime_s(&utc, &nowTime);
        return std::format("{:0>2}{:0>2}", utc.tm_hour, utc.tm_min);
    }
}

TEST(CdmStateHandlerTest, ResolvePresentation_TobtUsesInactiveGreenUntilReady) {
    const auto futureTobt = CurrentUtcHHMM(10);
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.tobt = futureTobt},
        CdmStateHandler::Field::Tobt
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, futureTobt);
    EXPECT_EQ(presentation.color, RGB(143, 216, 148));
}

TEST(CdmStateHandlerTest, ResolvePresentation_TobtUsesGreenInsideActiveWindow) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.tobt = CurrentUtcHHMM(-2)},
        CdmStateHandler::Field::Tobt
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.color, RGB(0, 192, 0));
}

TEST(CdmStateHandlerTest, ResolvePresentation_TobtUsesYellowAfterPlusFiveMinutes) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.tobt = CurrentUtcHHMM(-5)},
        CdmStateHandler::Field::Tobt
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.color, RGB(212, 214, 7));
}

TEST(CdmStateHandlerTest, ResolvePresentation_TobtStaysGreenWhileTsatIsStillActive) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.tobt = CurrentUtcHHMM(-6), .tsat = CurrentUtcHHMM(-2)},
        CdmStateHandler::Field::Tobt
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.color, RGB(0, 192, 0));
}

TEST(CdmStateHandlerTest, ResolvePresentation_TsacMismatchUsesOrange) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.tsat = "1200", .tsac = "1210"},
        CdmStateHandler::Field::Tsac
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "1210");
    EXPECT_EQ(presentation.color, RGB(212, 133, 46));
}

TEST(CdmStateHandlerTest, ResolvePresentation_ManualCtotFallsBackToOrangeDisplay) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.manual_ctot = "1234"},
        CdmStateHandler::Field::Ctot
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "1234");
    EXPECT_EQ(presentation.color, RGB(212, 133, 46));
}

TEST(CdmStateHandlerTest, ResolvePresentation_ComplyStatusUsesGreen) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.status = "COMPLY"},
        CdmStateHandler::Field::Status
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "COMPLY");
    EXPECT_EQ(presentation.color, RGB(0, 192, 0));
}

TEST(CdmStateHandlerTest, ResolvePresentation_EobtUsesRedWhenFlightSuspended) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.eobt = "1015", .status = "FLS/REGUL"},
        CdmStateHandler::Field::Eobt
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "1015");
    EXPECT_EQ(presentation.color, RGB(190, 0, 0));
}

TEST(CdmStateHandlerTest, ResolvePresentation_EobtUsesOrangeWhenFarFromTobt) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.eobt = "1015", .tobt = "1025"},
        CdmStateHandler::Field::Eobt
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.color, RGB(212, 133, 46));
}

TEST(CdmStateHandlerTest, ResolvePresentation_PhaseReturnsEmptyWithoutTobt) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{},
        CdmStateHandler::Field::Phase
    );

    ASSERT_FALSE(presentation.hasValue);
}

TEST(CdmStateHandlerTest, ResolvePresentation_PhaseUsesPWhenTobtInFuture) {
    const auto futureTobt = CurrentUtcHHMM(10);
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.tobt = futureTobt},
        CdmStateHandler::Field::Phase
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "P");
    EXPECT_EQ(presentation.color, RGB(0, 192, 0));
}

TEST(CdmStateHandlerTest, ResolvePresentation_PhaseUsesCWhenTsatSet) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.tobt = CurrentUtcHHMM(10), .tsat = CurrentUtcHHMM(10)},
        CdmStateHandler::Field::Phase
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "C");
    EXPECT_EQ(presentation.color, RGB(0, 192, 0));
}

TEST(CdmStateHandlerTest, ResolvePresentation_PhaseUsesCEvenWhenTsatExpired) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.tobt = CurrentUtcHHMM(-5), .tsat = CurrentUtcHHMM(-5)},
        CdmStateHandler::Field::Phase
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "C");
}

TEST(CdmStateHandlerTest, ResolvePresentation_PhaseReturnsEmptyWhenTobtPassedButNoTsat) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.tobt = CurrentUtcHHMM(-5)},
        CdmStateHandler::Field::Phase
    );

    EXPECT_FALSE(presentation.hasValue);
}

TEST(CdmStateHandlerTest, ResolvePresentation_PhaseUsesRedIWhenBackendMarksInvalid) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.tobt = "1015", .phase = "I"},
        CdmStateHandler::Field::Phase
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "I");
    EXPECT_EQ(presentation.color, RGB(190, 0, 0));
}

TEST(CdmStateHandlerTest, ResolvePresentation_TsatTobtDiffIncludesSignedGap) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.tobt = "1010", .tsat = "1025"},
        CdmStateHandler::Field::TsatTobtDiff
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "1025/15");
}

TEST(CdmStateHandlerTest, ResolvePresentation_TtgUsesSignedMinutesFromNow) {
    const auto futureTsat = CurrentUtcHHMM(10);
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.tsat = futureTsat},
        CdmStateHandler::Field::Ttg
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "-10");
}

TEST(CdmStateHandlerTest, ResolvePresentation_TsatUsesYellowAtPlusFourMinutes) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.tsat = CurrentUtcHHMM(-4)},
        CdmStateHandler::Field::Tsat
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.color, RGB(212, 214, 7));
}

TEST(CdmStateHandlerTest, ResolvePresentation_TtgUsesYellowAtPlusFourMinutes) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.tsat = CurrentUtcHHMM(-4)},
        CdmStateHandler::Field::Ttg
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.color, RGB(212, 214, 7));
}

TEST(CdmStateHandlerTest, ResolvePresentation_FlowMessageFallsBackToManualActivation) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.manual_ctot = "1130"},
        CdmStateHandler::Field::FlowMessage
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "MAN ACT");
    EXPECT_EQ(presentation.color, RGB(212, 214, 7));
}

TEST(CdmStateHandlerTest, ResolvePresentation_TobtSetByUsesRequestSource) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.tobt_confirmed_by = "PILOT"},
        CdmStateHandler::Field::TobtConfirmedBy
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "PILOT");
    EXPECT_EQ(presentation.color, RGB(0, 192, 0));
}

TEST(CdmStateHandlerTest, ResolvePresentation_TobtSetByUsesYellowWhenTsatIsPlusFourMinutes) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.tobt_confirmed_by = "ATC", .tsat = CurrentUtcHHMM(-4)},
        CdmStateHandler::Field::TobtConfirmedBy
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "ATC");
    EXPECT_EQ(presentation.color, RGB(0, 192, 0));
}

TEST(CdmStateHandlerTest, ResolvePresentation_ReadyStartupShowsRedUntilAsrt) {
    const auto waiting = CdmStateHandler::ResolvePresentation(
        CdmState{},
        CdmStateHandler::Field::ReadyStartup
    );
    const auto ready = CdmStateHandler::ResolvePresentation(
        CdmState{.asrt = "1015"},
        CdmStateHandler::Field::ReadyStartup
    );

    ASSERT_TRUE(waiting.hasValue);
    EXPECT_EQ(waiting.value, "RSTUP");
    EXPECT_EQ(waiting.color, RGB(190, 0, 0));

    ASSERT_TRUE(ready.hasValue);
    EXPECT_EQ(ready.value, "RSTUP");
    EXPECT_EQ(ready.color, RGB(0, 192, 0));
}

TEST(CdmStateHandlerTest, ResolvePresentation_ReadyStartupTreatsPilotRequestAsActive) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.status = "REQASRT/NULL"},
        CdmStateHandler::Field::ReadyStartup
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "RSTUP");
    EXPECT_EQ(presentation.color, RGB(0, 192, 0));
}

TEST(CdmStateHandlerTest, ResolvePresentation_AsatUsesGreenByDefault) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.asat = CurrentUtcHHMM(-2)},
        CdmStateHandler::Field::Asat
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.color, RGB(0, 192, 0));
}

TEST(CdmStateHandlerTest, ResolvePresentation_AsatUsesYellowWhenAged) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.asat = CurrentUtcHHMM(-6)},
        CdmStateHandler::Field::Asat
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.color, RGB(212, 214, 7));
}

TEST(CdmStateHandlerTest, ResolvePresentation_StartupApprovedGreysTobtAndTsat) {
    const auto tobtPresentation = CdmStateHandler::ResolvePresentation(
        CdmState{.tobt = "1010", .asat = "1005", .status = "PUSH"},
        CdmStateHandler::Field::Tobt
    );
    const auto tsatPresentation = CdmStateHandler::ResolvePresentation(
        CdmState{.tsat = "1020", .asat = "1005", .status = "PUSH"},
        CdmStateHandler::Field::Tsat
    );

    ASSERT_TRUE(tobtPresentation.hasValue);
    EXPECT_EQ(tobtPresentation.color, RGB(108, 108, 108));
    ASSERT_TRUE(tsatPresentation.hasValue);
    EXPECT_EQ(tsatPresentation.color, RGB(108, 108, 108));
}

TEST(CdmStateHandlerTest, ResolvePresentation_StartupApprovedKeepsAsatActive) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{.asat = CurrentUtcHHMM(-2), .status = "STUP"},
        CdmStateHandler::Field::Asat
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.color, RGB(0, 192, 0));
}

TEST(CdmStateHandlerTest, ResolvePresentation_TobtShowsPlaceholderWhenEmpty) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{},
        CdmStateHandler::Field::Tobt
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "----");
    EXPECT_EQ(presentation.color, RGB(108, 108, 108));
}

TEST(CdmStateHandlerTest, ResolvePresentation_EobtFallsBackToFlightplanEobt) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{},
        CdmStateHandler::Field::Eobt,
        "1430"
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "1430");
    EXPECT_EQ(presentation.color, RGB(182, 182, 182));
}

TEST(CdmStateHandlerTest, ResolvePresentation_EobtFallbackEmptyReturnsNoValue) {
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{},
        CdmStateHandler::Field::Eobt
    );

    EXPECT_FALSE(presentation.hasValue);
}

TEST(CdmStateHandlerTest, ResolvePresentation_PhaseFallsBackToFlightplanEobt) {
    const auto futureEobt = CurrentUtcHHMM(10);
    const auto presentation = CdmStateHandler::ResolvePresentation(
        CdmState{},
        CdmStateHandler::Field::Phase,
        futureEobt
    );

    ASSERT_TRUE(presentation.hasValue);
    EXPECT_EQ(presentation.value, "P");
}
