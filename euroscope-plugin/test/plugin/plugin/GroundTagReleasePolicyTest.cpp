#include <gtest/gtest.h>

#include "plugin/GroundTagReleasePolicy.h"

using FlightStrips::ParseRequestOnFrequency;
using FlightStrips::ShouldReleaseGroundTag;

TEST(GroundTagReleasePolicyTest, ParsesTopSkyRequestOnFrequencyPulse) {
    const auto request = ParseRequestOnFrequency("/ROF/SAS123/EKDK_B_CTR");
    ASSERT_TRUE(request.has_value());
    EXPECT_EQ(request->aircraftCallsign, "SAS123");
    EXPECT_EQ(request->controllerCallsign, "EKDK_B_CTR");
}

TEST(GroundTagReleasePolicyTest, RejectsMalformedPulsesAndOtherCoordinationMessages) {
    for (const auto* scratchPad : {"", "ROF", "/ROF/", "/ROF/SAS123", "/ROF//EKCH_APP",
                                  "/ROF/SAS123/", "/ROF/SAS123/EKCH_APP/", "/ROF/SAS123/EKCH_APP/extra",
                                  "/ROF/SAS 123/EKCH_APP", "/ROF/SAS123/EKCH_APP\n", "text/ROF/SAS123/EKCH_APP",
                                  "/LAM/ROF/SAS123", "/RJC/S/ROF/SAS123", "/rof?/SAS123/EKCH_APP"}) {
        EXPECT_FALSE(ParseRequestOnFrequency(scratchPad).has_value()) << scratchPad;
    }
}

TEST(GroundTagReleasePolicyTest, ReleasesTowerTagForApproachAndCentreRequests) {
    EXPECT_TRUE(ShouldReleaseGroundTag("EKCH_D_TWR", 4, "EKCH_APP", 5, false, true));
    EXPECT_TRUE(ShouldReleaseGroundTag("EKCH_A_TWR", 4, "EKDK_B_CTR", 6, false, true));
    EXPECT_TRUE(ShouldReleaseGroundTag("ekyt_twr", 4, "ekyt_2_app", 5, false, true));
}

TEST(GroundTagReleasePolicyTest, NeverReleasesAnotherControllersTagOrObserverTag) {
    EXPECT_FALSE(ShouldReleaseGroundTag("EKCH_TWR", 4, "EKCH_APP", 5, false, false));
    EXPECT_FALSE(ShouldReleaseGroundTag("EKCH_TWR", 4, "EKCH_APP", 5, true, true));
}

TEST(GroundTagReleasePolicyTest, PreservesNormalAirborneCoordinationAndOtherGroundPositions) {
    for (const auto* owner : {"EKCH_APP", "EKDK_CTR", "EKCH_GND", "EKCH_DEL", "EKCH_TWR_OBS", ""}) {
        EXPECT_FALSE(ShouldReleaseGroundTag(owner, 4, "EKDK_CTR", 6, false, true)) << owner;
    }
    EXPECT_FALSE(ShouldReleaseGroundTag("EKCH_APP", 5, "EKDK_CTR", 6, false, true));
    EXPECT_FALSE(ShouldReleaseGroundTag("EKDK_CTR", 6, "EKCH_APP", 5, false, true));
}

TEST(GroundTagReleasePolicyTest, RejectsNonAirborneRequestersAndFacilityMismatches) {
    for (const auto* requester : {"EKCH_TWR", "EKCH_GND", "EKCH_DEL", "EKCH_APP_OBS", "EKDK_FSS", ""}) {
        EXPECT_FALSE(ShouldReleaseGroundTag("EKCH_TWR", 4, requester, 5, false, true)) << requester;
    }
    EXPECT_FALSE(ShouldReleaseGroundTag("EKCH_TWR", 3, "EKCH_APP", 5, false, true));
    EXPECT_FALSE(ShouldReleaseGroundTag("EKCH_TWR", 4, "EKCH_APP", 1, false, true));
    EXPECT_FALSE(ShouldReleaseGroundTag("EKCH_TWR", 4, "EKCH_APP", 6, false, true));
    EXPECT_FALSE(ShouldReleaseGroundTag("EKCH_TWR", 4, "EKDK_CTR", 5, false, true));
}
