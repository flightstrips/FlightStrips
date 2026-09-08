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
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(), " sas123 ").text, "G02");
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(true, "fresh", -90), "SAS123").text, "L02");
}

TEST(AMANGainLossHandlerTest, HidesUnavailableGuidance) {
    EXPECT_EQ(AMANGainLossHandler::Resolve(false, Snapshot(), "SAS123").text, "----");
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(false), "SAS123").text, "----");
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(true, "stale"), "SAS123").text, "----");
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(true, "disconnected"), "SAS123").text, "----");
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(true, "fresh", std::nullopt), "SAS123").text, "----");
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, Snapshot(), "MISSING").text, "----");
}

TEST(AMANGainLossHandlerTest, DoesNotInferAValueFromMissingFlightIdentity) {
    auto snapshot = std::make_shared<FlightStrips::aman::GainLossSnapshot>();
    snapshot->authoritative = true;
    snapshot->flightIdByCallsign["SAS123"] = "missing-flight";
    EXPECT_EQ(AMANGainLossHandler::Resolve(true, snapshot, "SAS123").text, "----");
}
