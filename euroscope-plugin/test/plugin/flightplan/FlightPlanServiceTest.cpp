#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "flightplan/FlightPlanService.h"
#include "flightplan/FlightPlan.h"

using FlightStrips::flightplan::FlightPlanService;
using FlightStrips::flightplan::FlightPlan;

// ---------------------------------------------------------------------------
// GetEstimatedLandingTime
//
// The method adds (GetPositionPredictions().GetPointsNumber() * 60) seconds
// to the current UTC time and returns "HHMM".  For a default-constructed
// CFlightPlan the point count is 0, so the result must equal the current
// UTC time formatted as "HHMM".
// ---------------------------------------------------------------------------

TEST(FlightPlanServiceStaticTest, GetEstimatedLandingTime_ZeroPoints_ReturnsCurrentUtcHHMM) {
    EuroScopePlugIn::CFlightPlan fp;  // default: GetPositionPredictions().GetPointsNumber() == 0

    const auto result = FlightPlanService::GetEstimatedLandingTime(fp);

    // Result must be exactly 4 digits: HHMM
    ASSERT_EQ(result.size(), 4u);
    for (char c : result) {
        EXPECT_TRUE(std::isdigit(static_cast<unsigned char>(c)));
    }

    // Verify it matches current UTC HHMM (allow ±1 minute boundary skew)
    time_t now;
    tm ptm;
    time(&now);
    gmtime_s(&ptm, &now);
    const auto expected = std::format("{:0>2}{:0>2}", ptm.tm_hour, ptm.tm_min);

    // Accept either the computed minute or the next (due to potential second-boundary crossing)
    int resultMin = std::stoi(result.substr(2));
    int expectedMin = ptm.tm_min;
    int nextMin = (ptm.tm_min + 1) % 60;

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

// ---------------------------------------------------------------------------
// FlightPlan struct — basic field tests
// ---------------------------------------------------------------------------

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

TEST(FlightPlanStructTest, FieldAssignment_RoundTrips) {
    FlightPlan fp;
    fp.squawk = "7700";
    fp.stand  = "A1";
    fp.tracking_controller = "EK_APP";
    EXPECT_EQ(fp.squawk,               "7700");
    EXPECT_EQ(fp.stand,                "A1");
    EXPECT_EQ(fp.tracking_controller,  "EK_APP");
}
