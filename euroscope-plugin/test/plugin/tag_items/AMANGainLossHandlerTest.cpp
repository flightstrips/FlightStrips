#include "tag_items/AMANGainLossHandler.h"

#include <gtest/gtest.h>

using FlightStrips::TagItems::AMANGainLossHandler;

namespace {
    auto Snapshot(bool authoritative = true, std::string status = "fresh",
                  std::optional<long long> seconds = 90) -> std::shared_ptr<const FlightStrips::aman::GainLossSnapshot> {
        auto snapshot = std::make_shared<FlightStrips::aman::GainLossSnapshot>();
        snapshot->authoritative = authoritative;
        snapshot->flightIdByCallsign["SAS123"] = "flight-1";
        snapshot->byFlightId["flight-1"] = {
            .flightId = "flight-1", .callsign = "SAS123", .seconds = seconds, .dataStatus = std::move(status)
        };
        return snapshot;
    }
}

TEST(AMANGainLossHandlerTest, FormatsRequiredRoundingBoundaries) {
    const std::vector<std::pair<long long, std::string>> cases = {
        {-6001, "L99+"}, {-5970, "L99+"}, {-31, "L01"}, {-30, "L01"}, {-29, "=00"},
        {0, "=00"}, {29, "=00"}, {30, "G01"}, {31, "G01"}, {5970, "G99+"}, {6001, "G99+"}
    };
    for (const auto& [seconds, expected] : cases) {
        EXPECT_EQ(AMANGainLossHandler::Format(seconds), expected) << seconds;
    }
}

TEST(AMANGainLossHandlerTest, DisplaysFreshAuthoritativeValueByNormalizedCallsign) {
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(), " sas123 ", "EKCH", "EKCH").text, "G02");
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(true, "fresh", -90), "SAS123", "ekch", " EKCH ").text, "L02");
}

TEST(AMANGainLossHandlerTest, RendersNothingUnlessFlightIsArrivalForCurrentAirport) {
    EXPECT_TRUE(AMANGainLossHandler::Resolve(true, Snapshot(), "SAS123", "ESSA", "EKCH").text.empty());
    EXPECT_TRUE(AMANGainLossHandler::Resolve(true, Snapshot(), "SAS123", "EKCH", "").text.empty());
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
                      true, Snapshot(true, "fresh", seconds), "SAS123", "EKCH", "EKCH").color,
                  expected) << seconds;
    }
}

TEST(AMANGainLossHandlerTest, HidesUnavailableGuidance) {
    EXPECT_EQ(AMANGainLossHandler::Resolve(false, Snapshot(), "SAS123", "EKCH", "EKCH").text, "----");
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(false), "SAS123", "EKCH", "EKCH").text, "----");
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(true, "stale"), "SAS123", "EKCH", "EKCH").text, "----");
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(true, "disconnected"), "SAS123", "EKCH", "EKCH").text, "----");
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(true, "fresh", std::nullopt), "SAS123", "EKCH", "EKCH").text, "----");
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(), "MISSING", "EKCH", "EKCH").text, "----");
}

TEST(AMANGainLossHandlerTest, DoesNotInferAValueFromMissingFlightIdentity) {
    auto snapshot = std::make_shared<FlightStrips::aman::GainLossSnapshot>();
    snapshot->authoritative = true;
    snapshot->flightIdByCallsign["SAS123"] = "missing-flight";
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, snapshot, "SAS123", "EKCH", "EKCH").text, "----");
}
