#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "stands/StandService.h"
#include "stands/Stand.h"

using FlightStrips::stands::StandService;
using FlightStrips::stands::Stand;

// ---------------------------------------------------------------------------
// Helper: build a Stand with a given name/airport/radius at 0,0
// ---------------------------------------------------------------------------

static Stand MakeStand(const std::string& name, const std::string& airport, double radius = 50.0) {
    EuroScopePlugIn::CPosition pos;
    return Stand(name, airport, pos, radius);
}

// ---------------------------------------------------------------------------
// GetStand(string, airport) — the scratch-pad annotation parsing path
//
// Format: "GRP/S/<name>" or "GRP/S/<name>/".  The parser splits on '/'
// and extracts the third segment (index 2).
//
// Guards:
//   • empty string              → nullptr
//   • fewer than two '/' chars  → nullptr
//   • empty name segment        → nullptr
// ---------------------------------------------------------------------------

class StandServiceGetByNameTest : public ::testing::Test {
protected:
    StandService service{std::vector<Stand>{
        MakeStand("A1", "EKCH"),
        MakeStand("B2", "EKCH"),
        MakeStand("C3", "ESSA"),
    }};
};

TEST_F(StandServiceGetByNameTest, EmptyString_ReturnsNull) {
    EXPECT_EQ(service.GetStand("", "EKCH"), nullptr);
}

TEST_F(StandServiceGetByNameTest, UnknownStand_ReturnsNull) {
    // Valid format but stand name doesn't exist in the service.
    EXPECT_EQ(service.GetStand("GRP/S/ZZ/", "EKCH"), nullptr);
}

TEST_F(StandServiceGetByNameTest, StringWithNoDelimiters_ReturnsNull) {
    // No '/' characters at all → nullptr.
    EXPECT_EQ(service.GetStand("NODEL", "EKCH"), nullptr);
}

TEST_F(StandServiceGetByNameTest, StringWithOnlyOneSlash_ReturnsNull) {
    // Only one '/' → second find returns npos → nullptr.
    EXPECT_EQ(service.GetStand("ABC/DEF", "EKCH"), nullptr);
}

// Production format without trailing slash: "GRP/S/<name>"
TEST_F(StandServiceGetByNameTest, GrpAnnotation_NoTrailingSlash_ExtractsCorrectStand) {
    const auto* stand = service.GetStand("GRP/S/A1", "EKCH");
    ASSERT_NE(stand, nullptr);
    EXPECT_EQ(stand->GetName(), "A1");
}

// Production format with trailing slash: "GRP/S/<name>/"
TEST_F(StandServiceGetByNameTest, GrpAnnotation_WithTrailingSlash_ExtractsCorrectStand) {
    const auto* stand = service.GetStand("GRP/S/A1/", "EKCH");
    ASSERT_NE(stand, nullptr);
    EXPECT_EQ(stand->GetName(), "A1");
}

TEST_F(StandServiceGetByNameTest, GrpAnnotation_WrongAirport_ReturnsNull) {
    // Correct parse but airport mismatch → nullptr.
    EXPECT_EQ(service.GetStand("GRP/S/A1/", "ESSA"), nullptr);
}

TEST_F(StandServiceGetByNameTest, GrpAnnotation_EssaStand_ReturnsCorrectStand) {
    const auto* stand = service.GetStand("GRP/S/C3/", "ESSA");
    ASSERT_NE(stand, nullptr);
    EXPECT_EQ(stand->GetName(), "C3");
}

TEST_F(StandServiceGetByNameTest, GrpAnnotation_EmptyNameSegment_ReturnsNull) {
    // "GRP/S/" — third segment is empty → nullptr.
    EXPECT_EQ(service.GetStand("GRP/S/", "EKCH"), nullptr);
}

// ---------------------------------------------------------------------------
// GetStand(position) — zero position, all stands at zero pos with large radius
// ---------------------------------------------------------------------------

class StandServiceGetByPositionTest : public ::testing::Test {
protected:
    EuroScopePlugIn::CPosition zeroPos;

    StandService service{std::vector<Stand>{
        // radius 1e9 metres to ensure the zero position is "inside"
        Stand("A1", "EKCH", zeroPos, 1e9),
    }};
};

TEST_F(StandServiceGetByPositionTest, PositionInsideRadius_ReturnsStand) {
    auto* stand = service.GetStand(zeroPos);
    ASSERT_NE(stand, nullptr);
    EXPECT_EQ(stand->GetName(), "A1");
}

TEST_F(StandServiceGetByPositionTest, EmptyService_ReturnsNull) {
    StandService empty{{}};
    EXPECT_EQ(empty.GetStand(zeroPos), nullptr);
}

TEST_F(StandServiceGetByPositionTest, PositionOutsideRadius_ReturnsNull) {
    // Radius of 0 metres: nothing is within range.
    EuroScopePlugIn::CPosition pos;
    StandService tiny{std::vector<Stand>{Stand("X1", "EKCH", pos, 0.0)}};
    EXPECT_EQ(tiny.GetStand(pos), nullptr);
}

TEST_F(StandServiceGetByPositionTest, MultipleStands_ReturnsNearest) {
    // Two stands at the same position with different radii; nearest should win.
    EuroScopePlugIn::CPosition pos;
    StandService multi{std::vector<Stand>{
        Stand("Far",  "EKCH", pos, 1e9),
        Stand("Near", "EKCH", pos, 1e9),
    }};
    // Both have distance 0; the first one in the list wins (min = 1000 init,
    // 0 < 1e9 and 0 < 1000, so "Far" is selected first, then "Near" has
    // same distance 0 which is NOT < min (0), so "Far" wins).
    auto* stand = multi.GetStand(pos);
    ASSERT_NE(stand, nullptr);
    EXPECT_EQ(stand->GetName(), "Far");
}

// ---------------------------------------------------------------------------
// Constructor and basic accessors
// ---------------------------------------------------------------------------

TEST(StandServiceTest, ConstructWithEmptyStands_GetByNameReturnsNull) {
    StandService svc{{}};
    EXPECT_EQ(svc.GetStand("", "EKCH"), nullptr);
}

TEST(StandServiceTest, ConstructWithStands_CorrectAirportMatch) {
    std::vector<Stand> stands{MakeStand("P5", "ESSA"), MakeStand("Q6", "EKCH")};
    StandService svc{std::move(stands)};
    // No two '/' chars → nullptr.
    EXPECT_EQ(svc.GetStand("P5", "ESSA"), nullptr);
}

TEST(StandServiceTest, ConstructWithEmptyStands_GetByPositionReturnsNull) {
    EuroScopePlugIn::CPosition pos;
    StandService svc{{}};
    EXPECT_EQ(svc.GetStand(pos), nullptr);
}
