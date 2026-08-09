#include <gtest/gtest.h>

#include "plugin/ApproachHandoffPolicy.h"

using FlightStrips::ShouldAutoAssumeAndDropEkytApproachHandoff;
using FlightStrips::ShouldAutoDropEkytApproachAircraft;

TEST(ApproachHandoffPolicyTest, AcceptsTransferToEkytAppAtOrBelowFlightLevel125) {
    EXPECT_TRUE(ShouldAutoAssumeAndDropEkytApproachHandoff("EKYT_APP", "123.980", false, true, true, 12500));
    EXPECT_TRUE(ShouldAutoAssumeAndDropEkytApproachHandoff("ekyt_2_app", "120.705", false, true, true, 8000));
}

TEST(ApproachHandoffPolicyTest, WaitsUntilAircraftDescendsToFlightLevel125) {
    EXPECT_FALSE(ShouldAutoAssumeAndDropEkytApproachHandoff("EKYT_APP", "123.980", false, true, true, 12501));
    EXPECT_TRUE(ShouldAutoAssumeAndDropEkytApproachHandoff("EKYT_APP", "123.980", false, true, true, 12500));
}

TEST(ApproachHandoffPolicyTest, RejectsOtherPositionsAndNonHandoffStates) {
    EXPECT_FALSE(ShouldAutoAssumeAndDropEkytApproachHandoff("EKCH_APP", "123.980", false, true, true, 8000));
    EXPECT_FALSE(ShouldAutoAssumeAndDropEkytApproachHandoff("EKYT_APP", "123.980", false, false, true, 8000));
}

TEST(ApproachHandoffPolicyTest, RejectsOtherPrimaryFrequencies) {
    EXPECT_FALSE(ShouldAutoAssumeAndDropEkytApproachHandoff("EKYT_APP", "123.975", false, true, true, 8000));
    EXPECT_FALSE(ShouldAutoAssumeAndDropEkytApproachHandoff("EKYT_APP", "", false, true, true, 8000));
}

TEST(ApproachHandoffPolicyTest, RejectsObserversAndAircraftWithoutRadarAltitude) {
    EXPECT_FALSE(ShouldAutoAssumeAndDropEkytApproachHandoff("EKYT_APP", "120.705", true, true, true, 8000));
    EXPECT_FALSE(ShouldAutoAssumeAndDropEkytApproachHandoff("EKYT_APP", "120.705", false, true, false, 8000));
}

TEST(ApproachHandoffPolicyTest, DropsAircraftAssumedOnEligibleEkytPositions) {
    EXPECT_TRUE(ShouldAutoDropEkytApproachAircraft("EKYT_APP", "123.980", false, true, true));
    EXPECT_TRUE(ShouldAutoDropEkytApproachAircraft("ekyt_2_app", "120.705", false, true, true));
}

TEST(ApproachHandoffPolicyTest, DoesNotDropAircraftThatAreNotAssumedOrTrackedByMe) {
    EXPECT_FALSE(ShouldAutoDropEkytApproachAircraft("EKYT_APP", "123.980", false, false, true));
    EXPECT_FALSE(ShouldAutoDropEkytApproachAircraft("EKYT_APP", "123.980", false, true, false));
}

TEST(ApproachHandoffPolicyTest, DoesNotDropAssumedAircraftOutsideEligibleEkytPositions) {
    EXPECT_FALSE(ShouldAutoDropEkytApproachAircraft("EKCH_APP", "123.980", false, true, true));
    EXPECT_FALSE(ShouldAutoDropEkytApproachAircraft("EKYT_APP", "123.975", false, true, true));
    EXPECT_FALSE(ShouldAutoDropEkytApproachAircraft("EKYT_APP", "120.705", true, true, true));
}
