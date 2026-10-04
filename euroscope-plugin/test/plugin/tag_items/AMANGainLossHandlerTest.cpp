#include "tag_items/AMANGainLossHandler.h"

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

TEST(AMANGainLossHandlerTest, HidesUnavailableGuidance) {
    EXPECT_TRUE(AMANGainLossHandler::Resolve(true, nullptr, "SAS123", "EKCH", "EKCH", false, true, "EKDK_CTR").text.empty());
    EXPECT_EQ(AMANGainLossHandler::Resolve(false, Snapshot(), "SAS123", "EKCH", "EKCH", false, true, "EKDK_CTR").text, "");
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(false), "SAS123", "EKCH", "EKCH", false, true, "EKDK_CTR").text, "");
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(true, "stale"), "SAS123", "EKCH", "EKCH", false, true, "EKDK_CTR").text, "");
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(true, "disconnected"), "SAS123", "EKCH", "EKCH", false, true, "EKDK_CTR").text, "");
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(true, "fresh", std::nullopt), "SAS123", "EKCH", "EKCH", false, true, "EKDK_CTR").text, "");
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(), "MISSING", "EKCH", "EKCH", false, true, "EKDK_CTR").text, "");
}

TEST(AMANGainLossHandlerTest, DoesNotRequireBackendFlightIdentity) {
    auto snapshot = std::make_shared<FlightStrips::aman::GainLossSnapshot>();
    snapshot->authoritative = true;
    snapshot->byCallsign["SAS123"] = {.callsign = "SAS123", .seconds = 90, .dataStatus = "fresh"};
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, snapshot, "SAS123", "EKCH", "EKCH", false, true, "EKDK_CTR").text, "G02");
}

TEST(AMANGainLossHandlerTest, HidesGuidanceUnlessTrackedByMe) {
    EXPECT_TRUE(AMANGainLossHandler::Resolve(
        true, Snapshot(), "SAS123", "EKCH", "EKCH", false, false, "EKDK_CTR").text.empty());
    EXPECT_EQ(AMANGainLossHandler::Resolve(
        true, Snapshot(), "SAS123", "EKCH", "EKCH", false, true, "EKDK_CTR").text, "G02");
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
            EXPECT_TRUE(AMANGainLossHandler::Resolve(
                true, Snapshot(), "SAS123", "EKCH", "EKCH", false, trackedByMe, controller).text.empty())
                << controller;
            EXPECT_TRUE(AMANGainLossHandler::Resolve(
                false, nullptr, "SAS123", "EKCH", "EKCH", false, trackedByMe, controller).text.empty())
                << controller;
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
        for (const auto& snapshot : {Snapshot(false), Snapshot(true, "stale"), Snapshot(true, "disconnected"),
                                    Snapshot(true, "fresh", std::nullopt),
                                    std::shared_ptr<FlightStrips::aman::GainLossSnapshot>{}}) {
            EXPECT_EQ(AMANGainLossHandler::Resolve(
                true, snapshot, "SAS123", "EKCH", "EKCH", false, trackedByMe, "EKDK_FMP").text, "----");
        }
        EXPECT_EQ(AMANGainLossHandler::Resolve(
            false, Snapshot(), "SAS123", "EKCH", "EKCH", false, trackedByMe, "EKDK_FMP").text, "----");
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
