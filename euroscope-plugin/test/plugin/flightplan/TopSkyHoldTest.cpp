#include <gtest/gtest.h>
#include "flightplan/TopSkyHold.h"

using FlightStrips::flightplan::ParseTopSkyHoldAnnotation;
using FlightStrips::flightplan::ParseTopSkyHoldEat;
using FlightStrips::flightplan::TopSkyHold;

// Expected values were read off a live TopSky session:
//   SP=[/HOLD/OLPIB/]  A6=[h/OLPIB/h]   at assignment
//   SP=[]              A6=[h/OLPIB/h]   one millisecond later, and after

// ---------------------------------------------------------------------------
// ParseTopSkyHoldAnnotation — the durable state, in annotation 6
// ---------------------------------------------------------------------------

TEST(TopSkyHold, ParseAnnotation_ReadsEnrouteHold) {
    const auto hold = ParseTopSkyHoldAnnotation("h/OLPIB/h");
    EXPECT_TRUE(hold.active);
    EXPECT_FALSE(hold.tsa);
    EXPECT_EQ(hold.point, "OLPIB");
    EXPECT_EQ(hold.TypeName(), "enroute");
}

TEST(TopSkyHold, ParseAnnotation_ReadsTsaHold) {
    const auto hold = ParseTopSkyHoldAnnotation("t/EK-TSA-1/t");
    EXPECT_TRUE(hold.active);
    EXPECT_TRUE(hold.tsa);
    EXPECT_EQ(hold.point, "EK-TSA-1");
    EXPECT_EQ(hold.TypeName(), "tsa");
}

TEST(TopSkyHold, ParseAnnotation_EmptyIsNoHold) {
    EXPECT_FALSE(ParseTopSkyHoldAnnotation("").active);
}

// Annotation 6 is shared with GroundRadar's stand assignments.
TEST(TopSkyHold, ParseAnnotation_ReadsHoldAlongsideStand) {
    const auto hold = ParseTopSkyHoldAnnotation("GRP/S/A12h/ERNOV/h");
    EXPECT_TRUE(hold.active);
    EXPECT_EQ(hold.point, "ERNOV");
}

TEST(TopSkyHold, ParseAnnotation_StandAloneIsNoHold) {
    EXPECT_FALSE(ParseTopSkyHoldAnnotation("GRP/S/A12").active);
}

TEST(TopSkyHold, ParseAnnotation_ImplausiblePayloadIsNoHold) {
    EXPECT_FALSE(ParseTopSkyHoldAnnotation("h/THIS IS NOT A HOLDING POINT/h").active);
    EXPECT_FALSE(ParseTopSkyHoldAnnotation("h/A B/h").active);
    EXPECT_FALSE(ParseTopSkyHoldAnnotation("h//h").active);
}

TEST(TopSkyHold, ParseAnnotation_UnterminatedIsNoHold) {
    EXPECT_FALSE(ParseTopSkyHoldAnnotation("h/OLPIB").active);
}

// The scratch pad token is a transient pulse, never state.
TEST(TopSkyHold, ParseAnnotation_ScratchPadTokenIsNotState) {
    EXPECT_FALSE(ParseTopSkyHoldAnnotation("/HOLD/OLPIB/").active);
}

TEST(TopSkyHold, ParseAnnotation_ComparesByValue) {
    EXPECT_EQ(ParseTopSkyHoldAnnotation("h/OLPIB/h"), ParseTopSkyHoldAnnotation("h/OLPIB/h"));
    EXPECT_NE(ParseTopSkyHoldAnnotation("h/OLPIB/h"), ParseTopSkyHoldAnnotation("h/ERNOV/h"));
    EXPECT_NE(ParseTopSkyHoldAnnotation("h/OLPIB/h"), TopSkyHold{});
}

// ---------------------------------------------------------------------------
// ParseTopSkyHoldEat — the transient scratch pad pulse
// ---------------------------------------------------------------------------

TEST(TopSkyHold, ParseEat_ReadsExpectApproachTime) {
    EXPECT_EQ(ParseTopSkyHoldEat("/HOLD/OLPIB//HOLD_EAT/1422/"), "1422");
    EXPECT_EQ(ParseTopSkyHoldEat("/HOLD_EAT/1422/"), "1422");
}

TEST(TopSkyHold, ParseEat_AbsentYieldsNothing) {
    EXPECT_TRUE(ParseTopSkyHoldEat("/HOLD/OLPIB/").empty());
    EXPECT_TRUE(ParseTopSkyHoldEat("").empty());
    EXPECT_TRUE(ParseTopSkyHoldEat("GRP/S/A12").empty());
}

TEST(TopSkyHold, ParseEat_UnterminatedYieldsNothing) {
    EXPECT_TRUE(ParseTopSkyHoldEat("/HOLD_EAT/1422").empty());
}
