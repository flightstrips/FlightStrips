#include <gtest/gtest.h>
#include "flightplan/TopSkyHold.h"

using FlightStrips::flightplan::ParseTopSkyHoldAnnotation;
using FlightStrips::flightplan::ParseTopSkyHoldCommand;
using FlightStrips::flightplan::ParseTopSkyHoldEat;
using FlightStrips::flightplan::TopSkyHold;
using FlightStrips::flightplan::TopSkyHoldCommandType;
using FlightStrips::flightplan::BuildTopSkyHoldEatCommand;

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

// Scratch-pad commands have their own parser and must not be mistaken for the
// annotation format.
TEST(TopSkyHold, ParseAnnotation_ScratchPadTokenIsNotState) {
    EXPECT_FALSE(ParseTopSkyHoldAnnotation("/HOLD/OLPIB/").active);
}

TEST(TopSkyHold, ParseAnnotation_ComparesByValue) {
    EXPECT_EQ(ParseTopSkyHoldAnnotation("h/OLPIB/h"), ParseTopSkyHoldAnnotation("h/OLPIB/h"));
    EXPECT_NE(ParseTopSkyHoldAnnotation("h/OLPIB/h"), ParseTopSkyHoldAnnotation("h/ERNOV/h"));
    EXPECT_NE(ParseTopSkyHoldAnnotation("h/OLPIB/h"), TopSkyHold{});
}

// ---------------------------------------------------------------------------
// ParseTopSkyHoldCommand — the authoritative live scratch-pad protocol
// ---------------------------------------------------------------------------

TEST(TopSkyHold, ParseCommand_ReadsAssignment) {
    const auto command = ParseTopSkyHoldCommand("/HOLD/OLPIB/");
    EXPECT_EQ(command.type, TopSkyHoldCommandType::Assign);
    EXPECT_EQ(command.value, "OLPIB");
}

TEST(TopSkyHold, ParseCommand_ReadsCombinedAssignmentAndEat) {
    const auto command = ParseTopSkyHoldCommand("/HOLD/OLPIB//HOLD_EAT/1422/");
    EXPECT_EQ(command.type, TopSkyHoldCommandType::Assign);
    EXPECT_EQ(command.value, "OLPIB");
    EXPECT_EQ(command.eat, "1422");
}

TEST(TopSkyHold, ParseCommand_IgnoresAssignmentSuffix) {
    const auto command = ParseTopSkyHoldCommand("/HOLD/ROSBI/7");
    EXPECT_EQ(command.type, TopSkyHoldCommandType::Assign);
    EXPECT_EQ(command.value, "ROSBI");
}

TEST(TopSkyHold, ParseCommand_ReadsCancellationWithOrWithoutPoint) {
    EXPECT_EQ(ParseTopSkyHoldCommand("/XHOLD/OLPIB/").type, TopSkyHoldCommandType::Cancel);
    EXPECT_EQ(ParseTopSkyHoldCommand("/XHOLD/").type, TopSkyHoldCommandType::Cancel);
}

TEST(TopSkyHold, ParseCommand_ReadsAndValidatesEat) {
    const auto command = ParseTopSkyHoldCommand("/HOLD_EAT/1422/");
    EXPECT_EQ(command.type, TopSkyHoldCommandType::Eat);
    EXPECT_EQ(command.value, "1422");
    EXPECT_EQ(ParseTopSkyHoldCommand("/HOLD_EAT/2460/").type, TopSkyHoldCommandType::None);
}

TEST(TopSkyHold, ParseCommand_RejectsMalformedAssignment) {
    EXPECT_EQ(ParseTopSkyHoldCommand("/HOLD//").type, TopSkyHoldCommandType::None);
    EXPECT_EQ(ParseTopSkyHoldCommand("/HOLD/A B/").type, TopSkyHoldCommandType::None);
    EXPECT_EQ(ParseTopSkyHoldCommand("/HOLD/OLPIB").type, TopSkyHoldCommandType::None);
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

TEST(TopSkyHold, BuildsBackendEatPulseForMatchingEnrouteHold) {
	EXPECT_EQ(BuildTopSkyHoldEatCommand(TopSkyHold{true, false, "OLPIB"}, "OLPIB", "enroute", "1422"), "/HOLD_EAT/1422/");
}

TEST(TopSkyHold, RejectsMismatchedOrInvalidBackendEatPulse) {
	EXPECT_TRUE(BuildTopSkyHoldEatCommand(TopSkyHold{true, false, "ERNOV"}, "OLPIB", "enroute", "1422").empty());
	EXPECT_TRUE(BuildTopSkyHoldEatCommand(TopSkyHold{true, true, "OLPIB"}, "OLPIB", "enroute", "1422").empty());
	EXPECT_TRUE(BuildTopSkyHoldEatCommand(TopSkyHold{true, false, "OLPIB"}, "OLPIB", "enroute", "2460").empty());
}
