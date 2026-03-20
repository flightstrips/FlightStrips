#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "stands/Stand.h"

using FlightStrips::stands::Stand;

// ---------------------------------------------------------------------------
// Stand construction and accessors
// ---------------------------------------------------------------------------

class StandTest : public ::testing::Test {
protected:
    EuroScopePlugIn::CPosition MakePosition(double lat, double lon) {
        EuroScopePlugIn::CPosition pos;
        // Use a formatted string pair matching LoadFromStrings() expectations.
        // Format: "ddd.ddddd" — keep it simple with zero values for pure logic tests.
        char latStr[32], lonStr[32];
        snprintf(latStr, sizeof(latStr), "%.5f", lat);
        snprintf(lonStr, sizeof(lonStr), "%.5f", lon);
        pos.LoadFromStrings(lonStr, latStr);
        return pos;
    }
};

TEST_F(StandTest, GetName_ReturnsConstructedName) {
    EuroScopePlugIn::CPosition pos;
    Stand stand("A1", "EKCH", pos, 10.0);
    EXPECT_EQ(stand.GetName(), "A1");
}

TEST_F(StandTest, GetAirport_ReturnsConstructedAirport) {
    EuroScopePlugIn::CPosition pos;
    Stand stand("B2", "EKCH", pos, 5.0);
    EXPECT_EQ(stand.GetAirport(), "EKCH");
}

TEST_F(StandTest, GetRadius_ReturnsConstructedRadius) {
    EuroScopePlugIn::CPosition pos;
    Stand stand("C3", "EKCH", pos, 25.0);
    EXPECT_DOUBLE_EQ(stand.GetRadius(), 25.0);
}

TEST_F(StandTest, GetPosition_ReturnsConstructedPosition) {
    // GetPosition() returns the CPosition passed at construction.
    // We verify it is a CPosition (struct round-trip via default construction).
    EuroScopePlugIn::CPosition pos;
    Stand stand("D4", "EKCH", pos, 15.0);
    // The returned position is copy-equal to what was passed in.
    // CPosition has no operator==, so we check via DistanceTo which returns 0
    // for identical positions.
    EXPECT_DOUBLE_EQ(stand.GetPosition().DistanceTo(pos), 0.0);
}

TEST_F(StandTest, DifferentAirports_AreIndependent) {
    EuroScopePlugIn::CPosition pos;
    Stand s1("G1", "EKCH", pos, 20.0);
    Stand s2("G1", "ESSA", pos, 20.0);
    EXPECT_NE(s1.GetAirport(), s2.GetAirport());
    EXPECT_EQ(s1.GetName(), s2.GetName());
}

// ---------------------------------------------------------------------------
// Stand::FromLine — parsing from a stands file line
//
// FromLine layout (after stripping "STAND:"):
//   <airport> ':' <name> ':' <lat[14]> <sep[1]> <lon[14]> <sep[1]> <radius>
//
// Derived directly from the parsing code in Stand.cpp:
//   substr(6)         → strip "STAND:"
//   find_first_of(':')→ airport
//   find_first_of(':')→ name
//   substr(0,14)      → lat
//   substr(15,14)     → lon  (position 14 is a separator char)
//   substr(30)        → radius
// ---------------------------------------------------------------------------

// A valid line whose field widths exactly satisfy FromLine's fixed offsets.
// lat = "N055.37.03.360" (14 chars), sep = ':', lon = "E012.39.23.670" (14 chars), sep = ':', radius = "30"
static constexpr const char* kValidLine =
    "STAND:EKCH:A1:N055.37.03.360:E012.39.23.670:30";

TEST(StandFromLineTest, ParsesName) {
    Stand s = Stand::FromLine(kValidLine);
    EXPECT_EQ(s.GetName(), "A1");
}

TEST(StandFromLineTest, ParsesAirport) {
    Stand s = Stand::FromLine(kValidLine);
    EXPECT_EQ(s.GetAirport(), "EKCH");
}

TEST(StandFromLineTest, ParsesRadius) {
    Stand s = Stand::FromLine(kValidLine);
    EXPECT_DOUBLE_EQ(s.GetRadius(), 30.0);
}

TEST(StandFromLineTest, ParsesMultiCharName) {
    // Stand name with more than 2 characters
    Stand s = Stand::FromLine("STAND:EKCH:B14:N055.37.03.360:E012.39.23.670:25");
    EXPECT_EQ(s.GetName(), "B14");
    EXPECT_EQ(s.GetAirport(), "EKCH");
    EXPECT_DOUBLE_EQ(s.GetRadius(), 25.0);
}

TEST(StandFromLineTest, ParsesDifferentAirport) {
    Stand s = Stand::FromLine("STAND:ESSA:C5:N059.21.03.000:E017.55.08.000:40");
    EXPECT_EQ(s.GetAirport(), "ESSA");
    EXPECT_EQ(s.GetName(), "C5");
    EXPECT_DOUBLE_EQ(s.GetRadius(), 40.0);
}

TEST(StandFromLineTest, ParsesFractionalRadius) {
    Stand s = Stand::FromLine("STAND:EKCH:X1:N055.37.03.360:E012.39.23.670:12.5");
    EXPECT_DOUBLE_EQ(s.GetRadius(), 12.5);
}
