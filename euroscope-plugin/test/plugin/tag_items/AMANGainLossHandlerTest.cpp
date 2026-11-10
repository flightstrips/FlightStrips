#include "tag_items/AMANGainLossHandler.h"

#include <tuple>

#include <gtest/gtest.h>

using FlightStrips::TagItems::AMANGainLossHandler;

namespace {
    auto Snapshot(bool authoritative = true, std::string status = "fresh",
                  std::optional<long long> seconds = 90) -> std::shared_ptr<FlightStrips::aman::GainLossSnapshot> {
        auto snapshot = std::make_shared<FlightStrips::aman::GainLossSnapshot>();
        snapshot->authoritative = authoritative;
        snapshot->byCallsign["SAS123"] = {
            .flightId = "flight-1", .callsign = "SAS123", .seconds = seconds, .dataStatus = std::move(status)
        };
        return snapshot;
    }
}

TEST(AMANGainLossHandlerTest, FormatsRequiredRoundingBoundaries) {
    const std::vector<std::pair<long long, std::string>> cases = {
        {-6001, "L99+"}, {-5970, "L99+"}, {-31, "L01"}, {-30, "L01"}, {-29, "G00"},
        {0, "G00"}, {29, "G00"}, {30, "G01"}, {31, "G01"}, {5970, "G99+"}, {6001, "G99+"}
    };
    for (const auto& [seconds, expected] : cases) {
        EXPECT_EQ(AMANGainLossHandler::Format(seconds), expected) << seconds;
    }
}

TEST(AMANGainLossHandlerTest, DisplaysFreshAuthoritativeValueByNormalizedCallsign) {
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(), " sas123 ", "EKCH", "EKCH", false, true, "EKDK_CTR").text, "G02");
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(true, "fresh", -90), "SAS123", "ekch", " EKCH ", false, true, "EKDK_CTR").text, "L02");
}

TEST(AMANGainLossHandlerTest, RendersNothingUnlessFlightIsArrivalForCurrentAirport) {
    EXPECT_TRUE(AMANGainLossHandler::Resolve(true, Snapshot(), "SAS123", "ESSA", "EKCH", false, true, "EKDK_CTR").text.empty());
    EXPECT_TRUE(AMANGainLossHandler::Resolve(true, Snapshot(), "SAS123", "EKCH", "", false, true, "EKDK_CTR").text.empty());
}

TEST(AMANGainLossHandlerTest, RendersNothingWhileFlightHasAnActiveTopSkyHold) {
    EXPECT_TRUE(AMANGainLossHandler::Resolve(true, Snapshot(), "SAS123", "EKCH", "EKCH", true, true, "EKDK_CTR").text.empty());
}

TEST(AMANGainLossHandlerTest, ShowsHoldingCountdownFromExactlyFourMinutesBeforeEat) {
    const auto now = std::chrono::system_clock::time_point{std::chrono::seconds{1794355140}};
    auto snapshot = Snapshot(true, "fresh", -1800); // Landing guidance is not the holding countdown.
    for (const auto& [untilEat, expected] : std::vector<std::pair<long long, std::string>>{
             {300, ""}, {241, ""}, {240, "L04"}, {239, "L04"}, {180, "L03"},
             {60, "L01"}, {0, "G00"}, {-60, "G01"}}) {
        snapshot->byCallsign["SAS123"].holdingEatUnixSeconds = 1794355140 + untilEat;
        EXPECT_EQ(AMANGainLossHandler::Resolve(
            true, snapshot, "SAS123", "EKCH", "EKCH", true, true, "EKDK_CTR", now).text, expected)
            << untilEat;
    }
}

TEST(AMANGainLossHandlerTest, HoldingCountdownAdvancesAcrossMidnightWithoutANewSnapshot) {
    auto snapshot = Snapshot(true, "fresh", std::nullopt);
    snapshot->byCallsign["SAS123"].holdingEatUnixSeconds = 1794355380; // 2026-11-11 00:03 UTC.
    const auto beforeMidnight = std::chrono::system_clock::time_point{std::chrono::seconds{1794355140}};
    EXPECT_EQ(AMANGainLossHandler::Resolve(
        true, snapshot, "SAS123", "EKCH", "EKCH", true, true, "EKDK_CTR", beforeMidnight).text, "L04");
    EXPECT_EQ(AMANGainLossHandler::Resolve(
        true, snapshot, "SAS123", "EKCH", "EKCH", true, true, "EKDK_CTR", beforeMidnight + std::chrono::minutes{3}).text, "L01");
}

TEST(AMANGainLossHandlerTest, HoldingCountdownPreservesVisibilityAndDataStatusRules) {
    const auto now = std::chrono::system_clock::time_point{std::chrono::seconds{1794355140}};
    for (const std::string status : {"fresh", "stale", "disconnected"}) {
        for (const bool connected : {true, false}) {
            auto snapshot = Snapshot(true, status);
            snapshot->byCallsign["SAS123"].holdingEatUnixSeconds = 1794355380;
            const auto resolve = [&](const std::string& controller, bool tracked = true, const std::string& destination = "EKCH") {
                return AMANGainLossHandler::Resolve(connected, snapshot, "SAS123", destination, "EKCH", true, tracked, controller, now);
            };
            for (const std::string controller : {"EKDK_CTR", "EKDK_FMP", "ESMM_APP", "ESMM_CTR"}) {
                EXPECT_EQ(resolve(controller).text, "L04");
                EXPECT_EQ(resolve(controller).color, connected && status == "fresh" ? RGB(156, 0, 0) : RGB(160, 160, 160));
            }
            EXPECT_TRUE(resolve("EKDK_CTR", false).text.empty());
            EXPECT_EQ(resolve("EKDK_FMP", false).text, "L04");
            EXPECT_TRUE(resolve("EKCH_APP").text.empty());
            EXPECT_TRUE(resolve("EKDK_CTR", true, "ESSA").text.empty());
            snapshot->byCallsign["SAS123"].insideTMA = true;
            EXPECT_TRUE(resolve("EKDK_CTR").text.empty());
            snapshot->byCallsign["SAS123"].insideTMA = false;
            snapshot->airport = "ESSA";
            EXPECT_TRUE(resolve("EKDK_CTR").text.empty());
            snapshot->airport = "EKCH";
            snapshot->authoritative = false;
            EXPECT_TRUE(resolve("EKDK_CTR").text.empty());
        }
    }
}

TEST(AMANGainLossHandlerTest, RendersNothingWhileFlightIsInsideTMA) {
    auto snapshot = Snapshot();
    snapshot->byCallsign["SAS123"].insideTMA = true;
    EXPECT_TRUE(AMANGainLossHandler::Resolve(true, snapshot, "SAS123", "EKCH", "EKCH", false, true, "EKDK_CTR").text.empty());

    snapshot->byCallsign["SAS123"].seconds = std::nullopt;
    EXPECT_TRUE(AMANGainLossHandler::Resolve(true, snapshot, "SAS123", "EKCH", "EKCH", false, true, "EKDK_CTR").text.empty());
}

TEST(AMANGainLossHandlerTest, ColorsGuidanceByDisplayedLoseMinutes) {
    const std::vector<std::pair<long long, COLORREF>> cases = {
        {-6001, RGB(156, 0, 0)},
        {-210, RGB(156, 0, 0)},
        {-209, RGB(240, 225, 41)},
        {-30, RGB(240, 225, 41)},
        {-29, RGB(150, 215, 150)},
        {0, RGB(150, 215, 150)},
        {600, RGB(150, 215, 150)},
    };
    for (const auto& [seconds, expected] : cases) {
        EXPECT_EQ(AMANGainLossHandler::Resolve(
                      true, Snapshot(true, "fresh", seconds), "SAS123", "EKCH", "EKCH", false, true, "EKDK_CTR").color,
                  expected) << seconds;
    }
}

TEST(AMANGainLossHandlerTest, DisplaysPlaceholderWhenGuidanceHasNeverBeenAvailable) {
    for (const auto& snapshot : {Snapshot(false), Snapshot(true, "fresh", std::nullopt),
                                std::shared_ptr<FlightStrips::aman::GainLossSnapshot>{}}) {
        EXPECT_EQ(AMANGainLossHandler::Resolve(true, snapshot, "SAS123", "EKCH", "EKCH", false, true, "EKDK_CTR").text, "----");
    }
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(), "MISSING", "EKCH", "EKCH", false, true, "EKDK_CTR").text, "----");
}

TEST(AMANGainLossHandlerTest, MarksGuidanceOnlyWhenDataIsStaleOrDisconnected) {
    for (const std::string controller : {"EKDK_CTR", "EKDK_FMP"}) {
        for (const std::string status : {"fresh", "stale", "disconnected"}) {
            for (const bool connected : {false, true}) {
                for (const auto& [seconds, label, color] : std::vector<std::tuple<long long, std::string, COLORREF>>{
                         {60, "G01", RGB(150, 215, 150)},
                         {0, "G00", RGB(150, 215, 150)},
                         {-120, "L02", RGB(240, 225, 41)},
                         {-240, "L04", RGB(156, 0, 0)}}) {
                    const auto presentation = AMANGainLossHandler::Resolve(
                        connected, Snapshot(true, status, seconds), "SAS123", "EKCH", "EKCH", false, true, controller);
                    EXPECT_EQ(presentation.text, label);
                    EXPECT_EQ(presentation.color, connected && status == "fresh" ? color : RGB(160, 160, 160))
                        << controller << " " << status << " connected=" << connected;
                }
            }
        }
    }
}

TEST(AMANGainLossHandlerTest, KeepsUnavailableGuidanceGreyAcrossConnectionChanges) {
    for (const bool connected : {false, true}) {
        for (const auto& snapshot : {Snapshot(false), Snapshot(true, "fresh", std::nullopt),
                                    std::shared_ptr<FlightStrips::aman::GainLossSnapshot>{}}) {
            const auto presentation = AMANGainLossHandler::Resolve(
                connected, snapshot, "SAS123", "EKCH", "EKCH", false, true, "EKDK_CTR");
            EXPECT_EQ(presentation.text, "----");
            EXPECT_EQ(presentation.color, RGB(160, 160, 160));
        }
    }
}

TEST(AMANGainLossHandlerTest, DoesNotReuseGuidanceFromAnotherAirport) {
    auto snapshot = Snapshot();
    snapshot->airport = "ESSA";
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, snapshot, "SAS123", "EKCH", "EKCH", false, true, "EKDK_CTR").text, "----");
}

TEST(AMANGainLossHandlerTest, DoesNotRequireBackendFlightIdentity) {
    auto snapshot = std::make_shared<FlightStrips::aman::GainLossSnapshot>();
    snapshot->authoritative = true;
    snapshot->byCallsign["SAS123"] = {.callsign = "SAS123", .seconds = 90, .dataStatus = "fresh"};
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, snapshot, "SAS123", "EKCH", "EKCH", false, true, "EKDK_CTR").text, "G02");
}

TEST(AMANGainLossHandlerTest, HidesUnconcernedGuidanceUnlessTrackedByMe) {
    EXPECT_TRUE(AMANGainLossHandler::Resolve(
        true, Snapshot(), "SAS123", "EKCH", "EKCH", false, false, "EKDK_CTR").text.empty());
    EXPECT_EQ(AMANGainLossHandler::Resolve(
        true, Snapshot(), "SAS123", "EKCH", "EKCH", false, true, "EKDK_CTR").text, "G02");
}

TEST(AMANGainLossHandlerTest, DisplaysUnassumedGuidanceOnlyForAircraftEnteringOurSectors) {
    for (const std::string controller : {"EKDK_UC_CTR", "EKDK_UV_CTR", "ESMM_APP", "ESMM_S_CTR"}) {
        for (const auto& [entryMinutes, expected] : std::vector<std::pair<int, std::string>>{
                 {-1, ""}, {0, "G02"}, {1, "G02"}, {30, "G02"}}) {
            EXPECT_EQ(AMANGainLossHandler::Resolve(
                true, Snapshot(), "SAS123", "EKCH", "EKCH", false, false, controller, std::chrono::system_clock::now(), entryMinutes).text,
                expected) << controller << " entryMinutes=" << entryMinutes;
        }
    }
}

TEST(AMANGainLossHandlerTest, HidesUnconcernedPlaceholders) {
    for (const std::string controller : {"EKDK_UC_CTR", "EKDK_UV_CTR", "ESMM_APP", "ESMM_CTR"}) {
        EXPECT_TRUE(AMANGainLossHandler::Resolve(
            false, nullptr, "SAS123", "EKCH", "EKCH", false, false, controller, std::chrono::system_clock::now(), -1).text.empty())
            << controller;
    }
}

TEST(AMANGainLossHandlerTest, PreservesConcernedGuidancePresentation) {
    for (const int entryMinutes : {0, 15}) {
        for (const bool connected : {false, true}) {
            const auto presentation = AMANGainLossHandler::Resolve(
                connected, Snapshot(true, "fresh", -240), "SAS123", "EKCH", "EKCH",
                false, false, "EKDK_UC_CTR", std::chrono::system_clock::now(), entryMinutes);
            EXPECT_EQ(presentation.text, "L04");
            EXPECT_EQ(presentation.color, connected ? RGB(156, 0, 0) : RGB(160, 160, 160));
        }
        const auto unavailable = AMANGainLossHandler::Resolve(
            true, nullptr, "SAS123", "EKCH", "EKCH", false, false, "EKDK_UV_CTR", std::chrono::system_clock::now(), entryMinutes);
        EXPECT_EQ(unavailable.text, "----");
        EXPECT_EQ(unavailable.color, RGB(160, 160, 160));
    }
}

TEST(AMANGainLossHandlerTest, PreservesConcernedArrivalHoldAndTmaFilters) {
    for (const int entryMinutes : {0, 15}) {
        EXPECT_TRUE(AMANGainLossHandler::Resolve(
            true, Snapshot(), "SAS123", "ESSA", "EKCH", false, false, "EKDK_UC_CTR", std::chrono::system_clock::now(), entryMinutes).text.empty());
        EXPECT_TRUE(AMANGainLossHandler::Resolve(
            true, Snapshot(), "SAS123", "EKCH", "EKCH", true, false, "EKDK_UC_CTR", std::chrono::system_clock::now(), entryMinutes).text.empty());
        auto snapshot = Snapshot();
        snapshot->byCallsign["SAS123"].insideTMA = true;
        EXPECT_TRUE(AMANGainLossHandler::Resolve(
            true, snapshot, "SAS123", "EKCH", "EKCH", false, false, "EKDK_UC_CTR", std::chrono::system_clock::now(), entryMinutes).text.empty());
    }
}

TEST(AMANGainLossHandlerTest, PreservesFmpGuidanceForUntrackedAircraft) {
    EXPECT_EQ(AMANGainLossHandler::Resolve(
        true, Snapshot(), "SAS123", "EKCH", "EKCH", false, false, " ekdk_fmp ").text, "G02");
    EXPECT_TRUE(AMANGainLossHandler::Resolve(
        true, Snapshot(), "SAS123", "EKCH", "EKCH", false, false, "EKDK_FMP_OBS").text.empty());
}

TEST(AMANGainLossHandlerTest, HidesGuidanceAndPlaceholdersForOtherControllers) {
    for (const std::string controller : {"EKCH_APP", "EKCH_FMP", "ESMM_FMP", "ESMM_TWR", "ESMM_APP_OBS",
                                        "ESMMX_CTR", "EKDK", "EKDKX_CTR", ""}) {
        for (const bool trackedByMe : {false, true}) {
            for (const int entryMinutes : {-1, 0, 15}) {
                EXPECT_TRUE(AMANGainLossHandler::Resolve(
                    true, Snapshot(), "SAS123", "EKCH", "EKCH", false, trackedByMe, controller, std::chrono::system_clock::now(), entryMinutes).text.empty())
                    << controller;
                EXPECT_TRUE(AMANGainLossHandler::Resolve(
                    false, nullptr, "SAS123", "EKCH", "EKCH", false, trackedByMe, controller, std::chrono::system_clock::now(), entryMinutes).text.empty())
                    << controller;
            }
        }
    }
}

TEST(AMANGainLossHandlerTest, DisplaysGuidanceForEsmmApproachAndCenterWhenTrackedByMe) {
    for (const std::string controller : {"ESMM_APP", "ESMM_CTR", "ESMM_S_APP", " esmm_s_ctr "}) {
        EXPECT_EQ(AMANGainLossHandler::Resolve(
            true, Snapshot(), "SAS123", "EKCH", "EKCH", false, true, controller).text, "G02")
            << controller;
        EXPECT_TRUE(AMANGainLossHandler::Resolve(
            true, Snapshot(), "SAS123", "EKCH", "EKCH", false, false, controller).text.empty())
            << controller;
    }
}

TEST(AMANGainLossHandlerTest, NormalizesEkdkControllerCallsign) {
    EXPECT_EQ(AMANGainLossHandler::Resolve(
        true, Snapshot(), "SAS123", "EKCH", "EKCH", false, true, " ekdk_a_ctr ").text, "G02");
}

TEST(AMANGainLossHandlerTest, PreservesFmpUnavailablePlaceholders) {
    for (const bool trackedByMe : {false, true}) {
        for (const auto& snapshot : {Snapshot(false),
                                    Snapshot(true, "fresh", std::nullopt),
                                    std::shared_ptr<FlightStrips::aman::GainLossSnapshot>{}}) {
            EXPECT_EQ(AMANGainLossHandler::Resolve(
                true, snapshot, "SAS123", "EKCH", "EKCH", false, trackedByMe, "EKDK_FMP").text, "----");
        }
        EXPECT_EQ(AMANGainLossHandler::Resolve(
            false, Snapshot(), "SAS123", "EKCH", "EKCH", false, trackedByMe, "EKDK_FMP").text, "G02");
        EXPECT_EQ(AMANGainLossHandler::Resolve(
            true, Snapshot(), "MISSING", "EKCH", "EKCH", false, trackedByMe, "EKDK_FMP").text, "----");
    }
}

TEST(AMANGainLossHandlerTest, PreservesFmpArrivalHoldAndTmaFilters) {
    EXPECT_TRUE(AMANGainLossHandler::Resolve(
        true, Snapshot(), "SAS123", "ESSA", "EKCH", false, false, "EKDK_FMP").text.empty());
    EXPECT_TRUE(AMANGainLossHandler::Resolve(
        true, Snapshot(), "SAS123", "EKCH", "EKCH", true, false, "EKDK_FMP").text.empty());
    auto snapshot = Snapshot();
    snapshot->byCallsign["SAS123"].insideTMA = true;
    EXPECT_TRUE(AMANGainLossHandler::Resolve(
        true, snapshot, "SAS123", "EKCH", "EKCH", false, false, "EKDK_FMP").text.empty());
}
