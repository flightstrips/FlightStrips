#include <gtest/gtest.h>

#include "plugin/AirportResolution.h"

namespace {
    auto BuildPosition(const double latitude, const double longitude) -> EuroScopePlugIn::CPosition {
        EuroScopePlugIn::CPosition position;
        position.m_Latitude = latitude;
        position.m_Longitude = longitude;
        return position;
    }
}

TEST(AirportResolutionTest, ResolveAirportFromCallsign_MatchesConfiguredPrefix) {
    const CallsignAirportMap airportMap = {
        {"EKCH", {"EKCH", "EKDK"}},
    };

    EXPECT_EQ(FlightStrips::ResolveAirportFromCallsign("ekch_obs", airportMap), "EKCH");
    EXPECT_EQ(FlightStrips::ResolveAirportFromCallsign("EKDK_APP", airportMap), "EKCH");
}

TEST(AirportResolutionTest, ResolveAirportFromPosition_ReturnsClosestAirportWithinRange) {
    const auto controllerPosition = BuildPosition(55.6200, 12.6500);
    const std::vector<FlightStrips::configuration::AirportFallbackPoint> airports = {
        {"EKCH", 55.6181, 12.6560},
        {"ESSA", 59.6519, 17.9186},
    };

    const auto airport = FlightStrips::ResolveAirportFromPosition(controllerPosition, airports);
    ASSERT_TRUE(airport.has_value());
    EXPECT_EQ(*airport, "EKCH");
}

TEST(AirportResolutionTest, ResolveAirportFromPosition_ReturnsNulloptWhenOutOfRange) {
    const auto controllerPosition = BuildPosition(55.6200, 12.6500);
    const std::vector<FlightStrips::configuration::AirportFallbackPoint> airports = {
        {"ESSA", 59.6519, 17.9186},
    };

    EXPECT_FALSE(FlightStrips::ResolveAirportFromPosition(controllerPosition, airports).has_value());
}

TEST(AirportResolutionTest, HasSameAirportFallbackProbe_ReturnsTrueForUnchangedControllerPosition) {
    const std::optional<FlightStrips::AirportFallbackProbe> probe = FlightStrips::AirportFallbackProbe{
        .callsign = "FR_OBS",
        .latitude = 55.6179,
        .longitude = 12.6560,
    };

    EXPECT_TRUE(FlightStrips::HasSameAirportFallbackProbe(probe, "FR_OBS", BuildPosition(55.6179, 12.6560)));
}

TEST(AirportResolutionTest, HasSameAirportFallbackProbe_ReturnsFalseWhenCallsignOrPositionChanges) {
    const std::optional<FlightStrips::AirportFallbackProbe> probe = FlightStrips::AirportFallbackProbe{
        .callsign = "FR_OBS",
        .latitude = 55.6179,
        .longitude = 12.6560,
    };

    EXPECT_FALSE(FlightStrips::HasSameAirportFallbackProbe(probe, "EKCH_OBS", BuildPosition(55.6179, 12.6560)));
    EXPECT_FALSE(FlightStrips::HasSameAirportFallbackProbe(probe, "FR_OBS", BuildPosition(55.7000, 12.6560)));
}
