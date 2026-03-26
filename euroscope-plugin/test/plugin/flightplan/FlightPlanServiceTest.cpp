#include <gtest/gtest.h>

#include <memory>

#include "flightplan/FlightPlan.h"
#include "flightplan/FlightPlanService.h"

using FlightStrips::flightplan::FlightPlan;
using FlightStrips::flightplan::FlightPlanService;

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

TEST(FlightPlanStructTest, DefaultConstruction_CdmStateIsEmpty) {
    FlightPlan fp;
    EXPECT_EQ(fp.cdm.tobt, "");
    EXPECT_EQ(fp.cdm.tsat, "");
    EXPECT_EQ(fp.cdm.deice_type, "");
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

TEST(FlightPlanServiceStateTest, ApplyCdmUpdate_PopulatesBackendFields) {
    FlightPlanService service(
        std::shared_ptr<FlightStrips::websocket::WebSocketService>{},
        std::shared_ptr<FlightStrips::FlightStripsPlugin>{},
        std::shared_ptr<FlightStrips::stands::StandService>{},
        std::shared_ptr<FlightStrips::configuration::AppConfig>{}
    );

    service.ApplyCdmUpdate(CdmUpdateEvent{
        "EIN123", "1000", "1030", "1025", "PILOT", "1035", "1045", "1050", "1028", "1032", "1029", "REA", "1100", "M", "REGUL"
    });

    const auto* flightPlan = service.GetFlightPlan("EIN123");
    ASSERT_NE(flightPlan, nullptr);
    EXPECT_EQ(flightPlan->cdm.eobt, "1000");
    EXPECT_EQ(flightPlan->cdm.tobt, "1030");
    EXPECT_EQ(flightPlan->cdm.req_tobt, "1025");
    EXPECT_EQ(flightPlan->cdm.req_tobt_source, "PILOT");
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
        std::shared_ptr<FlightStrips::configuration::AppConfig>{}
    );

    BackendSyncCdmData syncData;
    syncData.tobt = "1040";
    syncData.req_tobt = "1035";
    syncData.req_tobt_source = "ATC";
    syncData.asat = "1042";
    syncData.deice_type = "H";
    syncData.ecfmp_id = "ATFM";

    service.ApplyBackendSyncCdm("SAS321", syncData);

    const auto* flightPlan = service.GetFlightPlan("SAS321");
    ASSERT_NE(flightPlan, nullptr);
    EXPECT_EQ(flightPlan->cdm.tobt, "1040");
    EXPECT_EQ(flightPlan->cdm.req_tobt, "1035");
    EXPECT_EQ(flightPlan->cdm.req_tobt_source, "ATC");
    EXPECT_EQ(flightPlan->cdm.asat, "1042");
    EXPECT_EQ(flightPlan->cdm.deice_type, "H");
    EXPECT_EQ(flightPlan->cdm.ecfmp_id, "ATFM");
}
