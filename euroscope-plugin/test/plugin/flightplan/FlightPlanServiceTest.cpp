#include <gtest/gtest.h>

#include <memory>

#include "flightplan/FlightPlan.h"
#include "flightplan/FlightPlanService.h"

using FlightStrips::flightplan::FlightPlan;
using FlightStrips::flightplan::FlightPlanService;
using FlightStrips::flightplan::ApplyHold;
using FlightStrips::flightplan::ApplyTopSkyHoldCommand;
using FlightStrips::flightplan::BuildTopSkyHoldEatCommand;
using FlightStrips::flightplan::ReconcileTopSkyHoldAnnotation;
using FlightStrips::flightplan::ShouldReportTopSkyHoldCommand;
using FlightStrips::flightplan::TopSkyHold;
using FlightStrips::flightplan::TopSkyHoldCommandType;

TEST(FlightPlanServiceStateTest, ApplyHoldCachesEatUntilReconnectSnapshot) {
    FlightPlan plan;
    const TopSkyHold hold{true, false, "OLPIB"};

    EXPECT_TRUE(ApplyHold(plan, hold, "1234"));
    EXPECT_EQ(plan.hold, "OLPIB");
    EXPECT_EQ(plan.hold_type, "enroute");
    EXPECT_EQ(plan.hold_eat, "1234");

    // The scratch-pad pulse is transient. A later annotation-only refresh must
    // preserve the cached EAT so a reconnect can resend the complete hold.
    EXPECT_FALSE(ApplyHold(plan, hold, ""));
    EXPECT_EQ(plan.hold_eat, "1234");
}

TEST(FlightPlanServiceStateTest, ScratchCommandsDriveHoldingState) {
    FlightPlan plan;

    EXPECT_TRUE(ApplyTopSkyHoldCommand(plan, {TopSkyHoldCommandType::Assign, "OLPIB"}));
    EXPECT_EQ(plan.hold, "OLPIB");
    EXPECT_EQ(plan.hold_type, "enroute");
    EXPECT_TRUE(plan.hold_eat.empty());

    EXPECT_TRUE(ApplyTopSkyHoldCommand(plan, {TopSkyHoldCommandType::Eat, "1422"}));
    EXPECT_EQ(plan.hold_eat, "1422");
    EXPECT_FALSE(ApplyTopSkyHoldCommand(plan, {TopSkyHoldCommandType::Assign, "OLPIB"}));
    EXPECT_EQ(plan.hold_eat, "1422");

    EXPECT_TRUE(ApplyTopSkyHoldCommand(plan, {TopSkyHoldCommandType::Assign, "ROSBI"}));
    EXPECT_EQ(plan.hold, "ROSBI");
    EXPECT_TRUE(plan.hold_eat.empty());

    EXPECT_TRUE(ApplyTopSkyHoldCommand(plan, {TopSkyHoldCommandType::Cancel, {}}));
    EXPECT_TRUE(plan.hold.empty());
    EXPECT_FALSE(ApplyTopSkyHoldCommand(plan, {TopSkyHoldCommandType::Cancel, {}}));
    EXPECT_FALSE(ReconcileTopSkyHoldAnnotation(plan, TopSkyHold{true, false, "OLPIB"}));
    EXPECT_TRUE(plan.hold.empty());
}

TEST(FlightPlanServiceStateTest, CombinedAssignmentAndEatUpdatesAtomically) {
    FlightPlan plan;
    EXPECT_TRUE(ApplyTopSkyHoldCommand(plan, {TopSkyHoldCommandType::Assign, "OLPIB", "1422"}));
    EXPECT_EQ(plan.hold, "OLPIB");
    EXPECT_EQ(plan.hold_type, "enroute");
    EXPECT_EQ(plan.hold_eat, "1422");
    EXPECT_FALSE(ApplyTopSkyHoldCommand(plan, {TopSkyHoldCommandType::Assign, "OLPIB", "1422"}));
}

TEST(FlightPlanServiceStateTest, EatWithoutKnownHoldDoesNotCreateOrClearState) {
    FlightPlan plan;
    EXPECT_FALSE(ApplyTopSkyHoldCommand(plan, {TopSkyHoldCommandType::Eat, "1422"}));
    EXPECT_TRUE(plan.hold.empty());
    EXPECT_TRUE(plan.hold_eat.empty());
}

TEST(FlightPlanServiceStateTest, EatDoesNotAttachToTsaHold) {
    FlightPlan plan;
    ASSERT_TRUE(ReconcileTopSkyHoldAnnotation(plan, TopSkyHold{true, true, "EK-TSA-1"}));
    EXPECT_FALSE(ApplyTopSkyHoldCommand(plan, {TopSkyHoldCommandType::Eat, "1422"}));
    EXPECT_TRUE(plan.hold_eat.empty());
}

TEST(FlightPlanServiceStateTest, MissingAnnotationDoesNotClearScratchState) {
    FlightPlan plan;
    ASSERT_TRUE(ApplyTopSkyHoldCommand(plan, {TopSkyHoldCommandType::Assign, "OLPIB"}));
    ASSERT_TRUE(ApplyTopSkyHoldCommand(plan, {TopSkyHoldCommandType::Eat, "1422"}));

    EXPECT_FALSE(ReconcileTopSkyHoldAnnotation(plan, {}));
    EXPECT_EQ(plan.hold, "OLPIB");
    EXPECT_EQ(plan.hold_eat, "1422");
}

TEST(FlightPlanServiceStateTest, ActiveAnnotationCanReconcileUnknownState) {
    FlightPlan plan;
    EXPECT_TRUE(ReconcileTopSkyHoldAnnotation(plan, TopSkyHold{true, false, "OLPIB"}));
    EXPECT_EQ(plan.hold, "OLPIB");
    EXPECT_EQ(plan.hold_type, "enroute");
}

TEST(FlightPlanServiceStateTest, TsaAnnotationReplacesEarlierCommandState) {
    FlightPlan plan;
    ASSERT_TRUE(ApplyTopSkyHoldCommand(plan, {TopSkyHoldCommandType::Assign, "OLPIB"}));

    EXPECT_TRUE(ReconcileTopSkyHoldAnnotation(plan, TopSkyHold{true, true, "EK-TSA-1"}));
    EXPECT_EQ(plan.hold, "EK-TSA-1");
    EXPECT_EQ(plan.hold_type, "tsa");
    EXPECT_FALSE(plan.hold_command_observed);
}

TEST(FlightPlanServiceStateTest, DuplicateAuthoritativeCommandsRemainReportable) {
    EXPECT_TRUE(ShouldReportTopSkyHoldCommand({TopSkyHoldCommandType::Assign, "OLPIB"}, false));
    EXPECT_TRUE(ShouldReportTopSkyHoldCommand({TopSkyHoldCommandType::Cancel, {}}, false));
    EXPECT_FALSE(ShouldReportTopSkyHoldCommand({TopSkyHoldCommandType::Eat, "1422"}, false));
    EXPECT_TRUE(ShouldReportTopSkyHoldCommand({TopSkyHoldCommandType::Eat, "1422"}, true));
}

TEST(FlightPlanServiceStaticTest, GetEstimatedLandingTime_ZeroPoints_ReturnsCurrentUtcHHMM) {
    EuroScopePlugIn::CFlightPlan fp;

    const auto result = FlightPlanService::GetEstimatedLandingTime(fp);

    ASSERT_EQ(result.size(), 4u);
    for (char c : result) {
        EXPECT_TRUE(std::isdigit(static_cast<unsigned char>(c)));
    }

    time_t now;
    tm ptm;
    time(&now);
    gmtime_s(&ptm, &now);
    const auto expected = std::format("{:0>2}{:0>2}", ptm.tm_hour, ptm.tm_min);

    const int resultMin = std::stoi(result.substr(2));
    const int nextMin = (ptm.tm_min + 1) % 60;
    EXPECT_TRUE(result == expected || resultMin == nextMin)
        << "result=" << result << " expected=" << expected;
}

TEST(FlightPlanServiceStaticTest, GetEstimatedLandingTime_ResultIsExactlyFourChars) {
    EuroScopePlugIn::CFlightPlan fp;
    const auto result = FlightPlanService::GetEstimatedLandingTime(fp);
    EXPECT_EQ(result.size(), 4u);
}

TEST(FlightPlanServiceStaticTest, GetEstimatedLandingTime_HourInRange) {
    EuroScopePlugIn::CFlightPlan fp;
    const auto result = FlightPlanService::GetEstimatedLandingTime(fp);
    ASSERT_EQ(result.size(), 4u);
    const int hour = std::stoi(result.substr(0, 2));
    EXPECT_GE(hour, 0);
    EXPECT_LE(hour, 23);
}

TEST(FlightPlanServiceStaticTest, GetEstimatedLandingTime_MinuteInRange) {
    EuroScopePlugIn::CFlightPlan fp;
    const auto result = FlightPlanService::GetEstimatedLandingTime(fp);
    ASSERT_EQ(result.size(), 4u);
    const int minute = std::stoi(result.substr(2, 2));
    EXPECT_GE(minute, 0);
    EXPECT_LE(minute, 59);
}

TEST(FlightPlanServiceStaticTest, GetEstimatedLandingTime_AllDigits) {
    EuroScopePlugIn::CFlightPlan fp;
    const auto result = FlightPlanService::GetEstimatedLandingTime(fp);
    for (char c : result) {
        EXPECT_TRUE(std::isdigit(static_cast<unsigned char>(c)))
            << "Non-digit character '" << c << "' in result: " << result;
    }
}

TEST(FlightPlanStructTest, DefaultConstruction_SquawkIsEmpty) {
    FlightPlan fp;
    EXPECT_EQ(fp.squawk, "");
}

TEST(FlightPlanStructTest, DefaultConstruction_StandIsEmpty) {
    FlightPlan fp;
    EXPECT_EQ(fp.stand, "");
}

TEST(FlightPlanStructTest, DefaultConstruction_TrackingControllerIsEmpty) {
    FlightPlan fp;
    EXPECT_EQ(fp.tracking_controller, "");
}

TEST(FlightPlanStructTest, DefaultConstruction_StripIsNotSynchronized) {
    FlightPlan fp;
    EXPECT_FALSE(fp.strip_synchronized);
}

TEST(FlightPlanServiceStaticTest, NormalizesDirectToFixWithoutCalculatingGeometry) {
    EXPECT_EQ(FlightPlanService::NormalizeDirectToFix(" kemax \t"), std::optional<std::string>{"KEMAX"});
    EXPECT_EQ(FlightPlanService::NormalizeDirectToFix("   "), std::nullopt);
    EXPECT_EQ(FlightPlanService::NormalizeDirectToFix(nullptr), std::nullopt);
}

TEST(FlightPlanServiceStaticTest, DirectToObservationTimestampIsUtcRfc3339) {
    const auto timestamp = FlightPlanService::CurrentUtcTimestamp();
    ASSERT_EQ(timestamp.size(), 20u);
    EXPECT_EQ(timestamp[4], '-');
    EXPECT_EQ(timestamp[7], '-');
    EXPECT_EQ(timestamp[10], 'T');
    EXPECT_EQ(timestamp[19], 'Z');
}

TEST(FlightPlanStructTest, DefaultConstruction_CdmStateIsEmpty) {
    FlightPlan fp;
    EXPECT_EQ(fp.cdm.tobt, "");
    EXPECT_EQ(fp.cdm.tsat, "");
    EXPECT_EQ(fp.cdm.deice_type, "");
}

TEST(FlightPlanStructTest, DefaultConstruction_PdcStateIsNotManaged) {
    FlightPlan fp;
    EXPECT_FALSE(fp.IsPdcCleared());
    EXPECT_FALSE(fp.IsPdcConfirmed());
    EXPECT_FALSE(fp.KeepsEuroScopeStripUncleared());
}

TEST(FlightPlanStructTest, FieldAssignment_RoundTrips) {
    FlightPlan fp;
    fp.squawk = "7700";
    fp.stand = "A1";
    fp.tracking_controller = "EK_APP";
    EXPECT_EQ(fp.squawk, "7700");
    EXPECT_EQ(fp.stand, "A1");
    EXPECT_EQ(fp.tracking_controller, "EK_APP");
}

TEST(FlightPlanStructTest, MarkRunwaySynced_InitializesRunwayState) {
    FlightPlan fp;

    EXPECT_FALSE(fp.HasRunwayChanged("04L"));
    fp.MarkRunwaySynced("04L");
    EXPECT_TRUE(fp.runway_initialized);
    EXPECT_EQ(fp.runway, "04L");
    EXPECT_FALSE(fp.strip_synchronized);
}

TEST(FlightPlanStructTest, HasRunwayChanged_DoesNotConsumeAssignedRunwayChange) {
    FlightPlan fp;
    fp.MarkRunwaySynced("04L");

    EXPECT_TRUE(fp.HasRunwayChanged("04R"));
    EXPECT_TRUE(fp.HasRunwayChanged("04R"));
    EXPECT_EQ(fp.runway, "04L");

    fp.MarkRunwaySynced("04R");
    EXPECT_EQ(fp.runway, "04R");
    EXPECT_FALSE(fp.HasRunwayChanged("04R"));
}

TEST(FlightPlanServiceStateTest, ApplyCdmUpdate_PopulatesBackendFields) {
    FlightPlanService service(
        std::shared_ptr<FlightStrips::websocket::WebSocketService>{},
        std::shared_ptr<FlightStrips::FlightStripsPlugin>{},
        std::shared_ptr<FlightStrips::stands::StandService>{},
        std::shared_ptr<FlightStrips::configuration::AppConfig>{},
        nullptr
    );

    CdmUpdateEvent update;
    update.callsign = "EIN123";
    update.eobt = "1000";
    update.tobt = "1030";
    update.tsat = "1035";
    update.ttot = "1045";
    update.ctot = "1050";
    update.asrt = "1028";
    update.tsac = "1032";
    update.asat = "1029";
    update.status = "REA";
    update.manual_ctot = "1100";
    update.deice_type = "M";
    update.ecfmp_id = "REGUL";
    service.ApplyCdmUpdate(update);

    const auto* flightPlan = service.GetFlightPlan("EIN123");
    ASSERT_NE(flightPlan, nullptr);
    EXPECT_EQ(flightPlan->cdm.eobt, "1000");
    EXPECT_EQ(flightPlan->cdm.tobt, "1030");
    EXPECT_EQ(flightPlan->cdm.tsat, "1035");
    EXPECT_EQ(flightPlan->cdm.ttot, "1045");
    EXPECT_EQ(flightPlan->cdm.ctot, "1050");
    EXPECT_EQ(flightPlan->cdm.asrt, "1028");
    EXPECT_EQ(flightPlan->cdm.tsac, "1032");
    EXPECT_EQ(flightPlan->cdm.asat, "1029");
    EXPECT_EQ(flightPlan->cdm.status, "REA");
    EXPECT_EQ(flightPlan->cdm.manual_ctot, "1100");
    EXPECT_EQ(flightPlan->cdm.deice_type, "M");
    EXPECT_EQ(flightPlan->cdm.ecfmp_id, "REGUL");
}

TEST(FlightPlanServiceStateTest, ApplyBackendSyncCdm_SeedsCdmState) {
    FlightPlanService service(
        std::shared_ptr<FlightStrips::websocket::WebSocketService>{},
        std::shared_ptr<FlightStrips::FlightStripsPlugin>{},
        std::shared_ptr<FlightStrips::stands::StandService>{},
        std::shared_ptr<FlightStrips::configuration::AppConfig>{},
        nullptr
    );

    BackendSyncCdmData syncData;
    syncData.tobt = "1040";
    syncData.asat = "1042";
    syncData.deice_type = "H";
    syncData.ecfmp_id = "ATFM";

    service.ApplyBackendSyncCdm("SAS321", syncData);

    const auto* flightPlan = service.GetFlightPlan("SAS321");
    ASSERT_NE(flightPlan, nullptr);
    EXPECT_EQ(flightPlan->cdm.tobt, "1040");
    EXPECT_EQ(flightPlan->cdm.asat, "1042");
    EXPECT_EQ(flightPlan->cdm.deice_type, "H");
    EXPECT_EQ(flightPlan->cdm.ecfmp_id, "ATFM");
}

TEST(FlightPlanServiceStateTest, BackendHoldEatReplaySurvivesLaterReconnectSnapshot) {
    FlightPlanService service(
        std::shared_ptr<FlightStrips::websocket::WebSocketService>{},
        std::shared_ptr<FlightStrips::FlightStripsPlugin>{},
        std::shared_ptr<FlightStrips::stands::StandService>{},
        std::shared_ptr<FlightStrips::configuration::AppConfig>{},
        nullptr
    );

    service.CacheBackendHoldEatReplay("SAS322", "OLPIB", "enroute", "1422");
    service.ApplyBackendSyncHold("SAS322", "OLPIB", "enroute", "1415");

    auto* flightPlan = service.GetFlightPlan("SAS322");
    ASSERT_NE(flightPlan, nullptr);
    EXPECT_EQ(flightPlan->hold, "OLPIB");
    EXPECT_EQ(flightPlan->hold_type, "enroute");
    EXPECT_EQ(flightPlan->hold_eat, "1415");
    ASSERT_TRUE(flightPlan->backend_hold_eat_replay.has_value());
    EXPECT_EQ(flightPlan->backend_hold_eat_replay->hold, "OLPIB");
    EXPECT_EQ(flightPlan->backend_hold_eat_replay->hold_type, "enroute");
    EXPECT_EQ(flightPlan->backend_hold_eat_replay->eat, "1422");
    EXPECT_EQ(
        BuildTopSkyHoldEatCommand(
            TopSkyHold{true, false, "OLPIB"}, flightPlan->backend_hold_eat_replay->hold,
            flightPlan->backend_hold_eat_replay->hold_type, flightPlan->backend_hold_eat_replay->eat),
        "/HOLD_EAT/1422/"
    );

    const auto command = FlightStrips::flightplan::TopSkyHoldCommand{TopSkyHoldCommandType::Eat, "1422"};
    const auto changed = ApplyTopSkyHoldCommand(*flightPlan, command);
    EXPECT_TRUE(changed);
    EXPECT_TRUE(ShouldReportTopSkyHoldCommand(command, changed));
    EXPECT_EQ(flightPlan->hold_eat, "1422");
    EXPECT_TRUE(flightPlan->backend_hold_eat_replay.has_value());
}

TEST(FlightPlanServiceStateTest, NewHoldCommandInvalidatesBackendEatReplay) {
    FlightPlan plan;
    plan.backend_hold_eat_replay = FlightStrips::flightplan::BackendHoldEatReplay{"OLPIB", "enroute", "1422"};

    EXPECT_TRUE(ApplyTopSkyHoldCommand(plan, {TopSkyHoldCommandType::Assign, "ROSBI"}));
    EXPECT_FALSE(plan.backend_hold_eat_replay.has_value());
}

TEST(FlightPlanServiceStateTest, DuplicateHoldCommandPreservesMatchingBackendEatReplay) {
    FlightPlan plan;
    ASSERT_TRUE(ApplyTopSkyHoldCommand(plan, {TopSkyHoldCommandType::Assign, "OLPIB"}));
    plan.backend_hold_eat_replay = FlightStrips::flightplan::BackendHoldEatReplay{"OLPIB", "enroute", "1422"};

    EXPECT_FALSE(ApplyTopSkyHoldCommand(plan, {TopSkyHoldCommandType::Assign, "OLPIB"}));
    ASSERT_TRUE(plan.backend_hold_eat_replay.has_value());
    EXPECT_EQ(plan.backend_hold_eat_replay->eat, "1422");
}

TEST(FlightPlanServiceStateTest, DifferentEatCommandInvalidatesBackendEatReplay) {
    FlightPlan plan;
    ASSERT_TRUE(ApplyTopSkyHoldCommand(plan, {TopSkyHoldCommandType::Assign, "OLPIB"}));
    plan.backend_hold_eat_replay = FlightStrips::flightplan::BackendHoldEatReplay{"OLPIB", "enroute", "1422"};

    EXPECT_TRUE(ApplyTopSkyHoldCommand(plan, {TopSkyHoldCommandType::Eat, "1430"}));
    EXPECT_FALSE(plan.backend_hold_eat_replay.has_value());
}

TEST(FlightPlanServiceStateTest, EmptyBackendEatWithdrawsReplayWithoutClearingObservedHold) {
    FlightPlanService service(
        std::shared_ptr<FlightStrips::websocket::WebSocketService>{},
        std::shared_ptr<FlightStrips::FlightStripsPlugin>{},
        std::shared_ptr<FlightStrips::stands::StandService>{},
        std::shared_ptr<FlightStrips::configuration::AppConfig>{},
        nullptr
    );
    service.ApplyBackendSyncHold("SAS324", "OLPIB", "enroute", "1422");
    service.CacheBackendHoldEatReplay("SAS324", "OLPIB", "enroute", "1422");

    service.CacheBackendHoldEatReplay("SAS324", "OLPIB", "enroute", "");

    const auto* flightPlan = service.GetFlightPlan("SAS324");
    ASSERT_NE(flightPlan, nullptr);
    EXPECT_FALSE(flightPlan->backend_hold_eat_replay.has_value());
    EXPECT_EQ(flightPlan->hold, "OLPIB");
    EXPECT_EQ(flightPlan->hold_eat, "1422");
}

TEST(FlightPlanServiceStateTest, ApplyBackendSyncHold_DoesNotOverwriteOfflineCommand) {
    FlightPlanService service(
        std::shared_ptr<FlightStrips::websocket::WebSocketService>{},
        std::shared_ptr<FlightStrips::FlightStripsPlugin>{},
        std::shared_ptr<FlightStrips::stands::StandService>{},
        std::shared_ptr<FlightStrips::configuration::AppConfig>{},
        nullptr
    );

    service.ApplyBackendSyncHold("SAS323", "OLPIB", "enroute", "1422");
    auto* flightPlan = service.GetFlightPlan("SAS323");
    ASSERT_NE(flightPlan, nullptr);
    ASSERT_TRUE(ApplyTopSkyHoldCommand(*flightPlan, {TopSkyHoldCommandType::Cancel, {}}));
    flightPlan->hold_command_pending = true;

    service.ApplyBackendSyncHold("SAS323", "OLPIB", "enroute", "1422");
    EXPECT_TRUE(flightPlan->hold.empty());
    EXPECT_TRUE(flightPlan->hold_type.empty());
    EXPECT_TRUE(flightPlan->hold_eat.empty());
    EXPECT_TRUE(flightPlan->hold_command_observed);
}

TEST(FlightPlanServiceStateTest, ApplyPdcStateChange_SeedsTrackedState) {
    FlightPlanService service(
        std::shared_ptr<FlightStrips::websocket::WebSocketService>{},
        std::shared_ptr<FlightStrips::FlightStripsPlugin>{},
        std::shared_ptr<FlightStrips::stands::StandService>{},
        std::shared_ptr<FlightStrips::configuration::AppConfig>{},
        nullptr
    );

    service.ApplyPdcStateChange("SAS321", "CLEARED");

    const auto* flightPlan = service.GetFlightPlan("SAS321");
    ASSERT_NE(flightPlan, nullptr);
    EXPECT_EQ(flightPlan->pdc_state, "CLEARED");
    EXPECT_TRUE(flightPlan->IsPdcCleared());
    EXPECT_TRUE(flightPlan->KeepsEuroScopeStripUncleared());
}
