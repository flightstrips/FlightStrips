#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include <memory>
#include <tuple>
#include <utility>
#include <vector>
#include "flightplan/FlightPlanService.h"
#include "flightplan/FlightPlan.h"

using FlightStrips::flightplan::FlightPlanService;
using FlightStrips::flightplan::FlightPlan;

namespace FlightStrips::flightplan {
    class FlightPlanServiceLocalCdmTestAccessor {
    public:
        static auto ParseFields(const std::string& annotation) -> std::tuple<std::string, std::string, std::string, std::string, std::string, std::string> {
            const auto snapshot = FlightPlanService::ParseLocalCdmAnnotation(annotation);
            return {
                snapshot.asrt,
                snapshot.tsac,
                snapshot.tobt,
                snapshot.tsat,
                snapshot.ttot,
                snapshot.manual_ctot
            };
        }

        static auto SplitSlashFields(const std::string& value) -> std::vector<std::string> {
            return FlightPlanService::SplitSlashFields(value);
        }

        static auto TrimWhitespace(std::string value) -> std::string {
            return FlightPlanService::TrimWhitespace(std::move(value));
        }

        static auto HasSendableValues(
            const std::string& tobt,
            const std::string& tsat,
            const std::string& ttot,
            const std::string& ctot,
            const std::string& manualCtot = ""
        ) -> bool {
            FlightPlanService::LocalCdmSnapshot snapshot;
            snapshot.tobt = tobt;
            snapshot.tsat = tsat;
            snapshot.ttot = ttot;
            snapshot.ctot = ctot;
            snapshot.manual_ctot = manualCtot;
            return snapshot.HasSendableValues();
        }

        static auto HasActiveObservationWindow(const FlightPlanService& service, const std::string& callsign) -> bool {
            return service.HasActiveLocalCdmObservationWindow(callsign);
        }

        static void RefreshObservationWindow(FlightPlanService& service, const std::string& callsign, const std::string& reason) {
            service.RefreshLocalCdmObservationWindow(callsign, reason);
        }

        static void ForgetLocalCdmState(FlightPlanService& service, const std::string& callsign) {
            service.ForgetLocalCdmState(callsign);
        }

        static auto BuildReadyAnnotation(const std::string& current, const std::string& hhmm) -> std::string {
            return FlightPlanService::BuildReadyAnnotation(current, hhmm);
        }
    };
}

using FlightStrips::flightplan::FlightPlanServiceLocalCdmTestAccessor;

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

TEST(FlightPlanServiceLocalCdmTest, ParseLocalCdmAnnotation_MapsSlashSeparatedFields) {
    const auto [asrt, tsac, tobt, tsat, ttot, manualCtot] =
        FlightPlanServiceLocalCdmTestAccessor::ParseFields("ASRT/TSAC/1210/1214/1218/deIce/ecfmp/1222");

    EXPECT_EQ(asrt, "ASRT");
    EXPECT_EQ(tsac, "TSAC");
    EXPECT_EQ(tobt, "1210");
    EXPECT_EQ(tsat, "1214");
    EXPECT_EQ(ttot, "1218");
    EXPECT_EQ(manualCtot, "1222");
}

TEST(FlightPlanServiceLocalCdmTest, ParseLocalCdmAnnotation_PreservesEmptyIntermediateFields) {
    const auto [asrt, tsac, tobt, tsat, ttot, manualCtot] =
        FlightPlanServiceLocalCdmTestAccessor::ParseFields("ASRT//1210///deIce/ecfmp/");

    EXPECT_EQ(asrt, "ASRT");
    EXPECT_EQ(tsac, "");
    EXPECT_EQ(tobt, "1210");
    EXPECT_EQ(tsat, "");
    EXPECT_EQ(ttot, "");
    EXPECT_EQ(manualCtot, "");
}

TEST(FlightPlanServiceLocalCdmTest, SplitSlashFields_PreservesTrailingEmptySegment) {
    const auto fields = FlightPlanServiceLocalCdmTestAccessor::SplitSlashFields("A/B/");

    ASSERT_EQ(fields.size(), 3u);
    EXPECT_EQ(fields[0], "A");
    EXPECT_EQ(fields[1], "B");
    EXPECT_EQ(fields[2], "");
}

TEST(FlightPlanServiceLocalCdmTest, TrimWhitespace_RemovesLeadingAndTrailingWhitespace) {
    EXPECT_EQ(FlightPlanServiceLocalCdmTestAccessor::TrimWhitespace("  1210 \t"), "1210");
}

TEST(FlightPlanServiceLocalCdmTest, LocalCdmSnapshotHasSendableValues_OnlyForTimingFields) {
    EXPECT_TRUE(FlightPlanServiceLocalCdmTestAccessor::HasSendableValues("1210", "", "", ""));
    EXPECT_TRUE(FlightPlanServiceLocalCdmTestAccessor::HasSendableValues("", "1214", "", ""));
    EXPECT_TRUE(FlightPlanServiceLocalCdmTestAccessor::HasSendableValues("", "", "1218", ""));
    EXPECT_TRUE(FlightPlanServiceLocalCdmTestAccessor::HasSendableValues("", "", "", "1222"));
    EXPECT_FALSE(FlightPlanServiceLocalCdmTestAccessor::HasSendableValues("", "", "", "", "1222"));
}

TEST(FlightPlanServiceLocalCdmTest, ObservationWindow_IsInactiveUntilBackendRequestStartsIt) {
    FlightPlanService service(
        std::shared_ptr<FlightStrips::websocket::WebSocketService>{},
        std::shared_ptr<FlightStrips::FlightStripsPlugin>{},
        std::shared_ptr<FlightStrips::stands::StandService>{},
        std::shared_ptr<FlightStrips::configuration::AppConfig>{}
    );

    EXPECT_FALSE(FlightPlanServiceLocalCdmTestAccessor::HasActiveObservationWindow(service, "SAS123"));
}

TEST(FlightPlanServiceLocalCdmTest, ObservationWindow_BecomesActiveOnlyAfterExplicitRefresh) {
    FlightPlanService service(
        std::shared_ptr<FlightStrips::websocket::WebSocketService>{},
        std::shared_ptr<FlightStrips::FlightStripsPlugin>{},
        std::shared_ptr<FlightStrips::stands::StandService>{},
        std::shared_ptr<FlightStrips::configuration::AppConfig>{}
    );

    FlightPlanServiceLocalCdmTestAccessor::RefreshObservationWindow(service, "SAS123", "ready-request");

    EXPECT_TRUE(FlightPlanServiceLocalCdmTestAccessor::HasActiveObservationWindow(service, "SAS123"));
}

TEST(FlightPlanServiceLocalCdmTest, ForgetLocalCdmState_ClearsRequestedObservationWindow) {
    FlightPlanService service(
        std::shared_ptr<FlightStrips::websocket::WebSocketService>{},
        std::shared_ptr<FlightStrips::FlightStripsPlugin>{},
        std::shared_ptr<FlightStrips::stands::StandService>{},
        std::shared_ptr<FlightStrips::configuration::AppConfig>{}
    );

    FlightPlanServiceLocalCdmTestAccessor::RefreshObservationWindow(service, "SAS123", "ready-request");
    ASSERT_TRUE(FlightPlanServiceLocalCdmTestAccessor::HasActiveObservationWindow(service, "SAS123"));

    FlightPlanServiceLocalCdmTestAccessor::ForgetLocalCdmState(service, "SAS123");

    EXPECT_FALSE(FlightPlanServiceLocalCdmTestAccessor::HasActiveObservationWindow(service, "SAS123"));
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

// ---------------------------------------------------------------------------
// BuildReadyAnnotation
// ---------------------------------------------------------------------------

TEST(BuildReadyAnnotationTest, SetsTobtOnEmptyAnnotation) {
    const auto result = FlightPlanServiceLocalCdmTestAccessor::BuildReadyAnnotation("", "1430");

    const auto [asrt, tsac, tobt, tsat, ttot, manualCtot] =
        FlightPlanServiceLocalCdmTestAccessor::ParseFields(result);

    EXPECT_EQ(tobt, "1430");
}

TEST(BuildReadyAnnotationTest, PreservesExistingFieldsOtherThanTobt) {
    const auto result = FlightPlanServiceLocalCdmTestAccessor::BuildReadyAnnotation(
        "existing_asrt/existing_tsac/old_tobt/1435/1440/deice/ecfmp/1/", "1430");

    const auto [asrt, tsac, tobt, tsat, ttot, manualCtot] =
        FlightPlanServiceLocalCdmTestAccessor::ParseFields(result);

    EXPECT_EQ(asrt,       "existing_asrt");
    EXPECT_EQ(tsac,       "existing_tsac");
    EXPECT_EQ(tobt,       "1430");
    EXPECT_EQ(tsat,       "1435");
    EXPECT_EQ(ttot,       "1440");
    EXPECT_EQ(manualCtot, "1");
}

TEST(BuildReadyAnnotationTest, OutputHasExactlyEightSlashTerminatedFields) {
    const auto result = FlightPlanServiceLocalCdmTestAccessor::BuildReadyAnnotation("", "1430");

    // Eight fields each followed by '/' → exactly 8 slashes, trailing slash included
    const auto slashCount = std::count(result.begin(), result.end(), '/');
    EXPECT_EQ(slashCount, 8);
    EXPECT_EQ(result.back(), '/');
}

TEST(BuildReadyAnnotationTest, PadsShortAnnotationToEightFields) {
    // Only 3 fields provided — remaining must be padded with empty strings
    const auto result = FlightPlanServiceLocalCdmTestAccessor::BuildReadyAnnotation(
        "old_asrt/tsac/old_tobt/", "1430");

    const auto fields = FlightPlanServiceLocalCdmTestAccessor::SplitSlashFields(result);
    // SplitSlashFields on "a/b/.../h/" yields 9 elements (trailing empty after last '/')
    ASSERT_GE(fields.size(), 8u);
    EXPECT_EQ(fields[0], "old_asrt");  // ASRT overwritten
    EXPECT_EQ(fields[1], "tsac");  // preserved
    EXPECT_EQ(fields[2], "1430");  // TOBT overwritten
    EXPECT_EQ(fields[3], "");      // padded
}

TEST(BuildReadyAnnotationTest, OverwritesExistingTobt) {
    const auto result = FlightPlanServiceLocalCdmTestAccessor::BuildReadyAnnotation(
        "0900/tsac/0900/tsat/ttot/deice/ecfmp/0/", "1430");

    const auto [asrt, tsac, tobt, tsat, ttot, manualCtot] =
        FlightPlanServiceLocalCdmTestAccessor::ParseFields(result);

    EXPECT_EQ(tobt, "1430");
}
