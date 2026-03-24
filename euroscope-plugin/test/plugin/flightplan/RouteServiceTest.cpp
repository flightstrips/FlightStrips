#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "flightplan/RouteService.h"
#include "plugin/FlightStripsPluginInterface.h"

using FlightStrips::flightplan::RouteService;
using FlightStrips::FlightStripsPluginInterface;
using FlightStrips::Sid;

// ---------------------------------------------------------------------------
// FakePlugin — returns a fixed set of SIDs for any airport
// ---------------------------------------------------------------------------

class FakePlugin final : public FlightStripsPluginInterface {
public:
    std::vector<Sid> GetSids(const std::string& /*airport*/) override {
        return {
            Sid("LANGO2C", "22R"),
            Sid("NEXEN2C", "22R"),
            Sid("SALLO1C", "22R"),
        };
    }
};

// FakePlugin that returns no SIDs — used to test guard-on-empty-SIDs paths.
class EmptySidsPlugin final : public FlightStripsPluginInterface {
public:
    std::vector<Sid> GetSids(const std::string& /*airport*/) override {
        return {};
    }
};

// ---------------------------------------------------------------------------
// SetSid tests (migrated from doctest SUBCASE style)
// ---------------------------------------------------------------------------

class SetSidTest : public ::testing::Test {
protected:
    std::shared_ptr<FakePlugin> plugin = std::make_shared<FakePlugin>();
    RouteService service{plugin};
    const std::string newSid = "LANGO2C";
    const std::string airport = "EKCH";
};

TEST_F(SetSidTest, ReplacesBaseSid) {
    std::string route = "NEXEN T503 GIMRU DCT MICOS DCT RIMET/N0495F390 T157 KERAX KERAX3A";
    service.SetSid(route, newSid, airport);
    EXPECT_EQ(route, "LANGO2C T503 GIMRU DCT MICOS DCT RIMET/N0495F390 T157 KERAX KERAX3A");
}

TEST_F(SetSidTest, ReplacesFullSid) {
    std::string route = "NEXEN2C T503 GIMRU DCT MICOS DCT RIMET/N0495F390 T157 KERAX KERAX3A";
    service.SetSid(route, newSid, airport);
    EXPECT_EQ(route, "LANGO2C T503 GIMRU DCT MICOS DCT RIMET/N0495F390 T157 KERAX KERAX3A");
}

TEST_F(SetSidTest, ReplacesWithAirportPrefix) {
    std::string route = "EKCH/22R NEXEN2C T503 GIMRU DCT MICOS DCT RIMET/N0495F390 T157 KERAX KERAX3A";
    service.SetSid(route, newSid, airport);
    EXPECT_EQ(route, "LANGO2C T503 GIMRU DCT MICOS DCT RIMET/N0495F390 T157 KERAX KERAX3A");
}

TEST_F(SetSidTest, ReplacesWithAirportPrefixLowerCase) {
    std::string route = "ekch/22r nexen2c t503 gimru dct micos dct rimet/n0495f390 t157 kerax kerax3a";
    service.SetSid(route, newSid, airport);
    EXPECT_EQ(route, "LANGO2C T503 GIMRU DCT MICOS DCT RIMET/N0495F390 T157 KERAX KERAX3A");
}

TEST_F(SetSidTest, InsertsWhenNoSidPresent) {
    std::string route = "T503 GIMRU DCT MICOS DCT RIMET/N0495F390 T157 KERAX KERAX3A";
    service.SetSid(route, newSid, airport);
    EXPECT_EQ(route, "LANGO2C T503 GIMRU DCT MICOS DCT RIMET/N0495F390 T157 KERAX KERAX3A");
}

TEST_F(SetSidTest, AlreadyCorrectSidDoesNothing) {
    std::string route = "LANGO2C LANGO T503 GIMRU DCT MICOS DCT RIMET/N0495F390 T157 KERAX KERAX3A";
    const std::string original = route;
    service.SetSid(route, newSid, airport);
    EXPECT_EQ(route, original);
}

TEST_F(SetSidTest, EmptyRoute_SetsJustTheSid) {
    // When the route is empty (no tokens), the SID becomes the entire route.
    std::string route;
    service.SetSid(route, newSid, airport);
    EXPECT_EQ(route, "LANGO2C");
}

// ---------------------------------------------------------------------------
// SetSid guard tests — empty airport or no SIDs → route unchanged
// ---------------------------------------------------------------------------

class SetSidGuardTest : public ::testing::Test {
protected:
    std::shared_ptr<FakePlugin>     fakePlugin  = std::make_shared<FakePlugin>();
    std::shared_ptr<EmptySidsPlugin> emptyPlugin = std::make_shared<EmptySidsPlugin>();
};

TEST_F(SetSidGuardTest, EmptyAirport_RouteUnchanged) {
    RouteService svc{fakePlugin};
    std::string route = "NEXEN T503 GIMRU";
    const std::string original = route;
    svc.SetSid(route, "LANGO2C", "");  // empty airport → early return
    EXPECT_EQ(route, original);
}

TEST_F(SetSidGuardTest, NoSidsFromPlugin_RouteUnchanged) {
    RouteService svc{emptyPlugin};
    std::string route = "NEXEN T503 GIMRU";
    const std::string original = route;
    svc.SetSid(route, "LANGO2C", "EKCH");  // plugin returns no SIDs → early return
    EXPECT_EQ(route, original);
}

TEST_F(SetSidGuardTest, EmptyAirport_SetDepartureRunway_RouteUnchanged) {
    RouteService svc{fakePlugin};
    std::string route = "NEXEN T503 GIMRU";
    const std::string original = route;
    svc.SetDepartureRunway(route, "22L", "");  // empty airport → early return
    EXPECT_EQ(route, original);
}

TEST_F(SetSidGuardTest, NoSidsFromPlugin_SetDepartureRunway_RouteUnchanged) {
    RouteService svc{emptyPlugin};
    std::string route = "NEXEN T503 GIMRU";
    const std::string original = route;
    svc.SetDepartureRunway(route, "22L", "EKCH");  // no SIDs → early return
    EXPECT_EQ(route, original);
}

// ---------------------------------------------------------------------------
// SetDepartureRunway tests (migrated from doctest SUBCASE style)
// ---------------------------------------------------------------------------

class SetDepartureRunwayTest : public ::testing::Test {
protected:
    std::shared_ptr<FakePlugin> plugin = std::make_shared<FakePlugin>();
    RouteService service{plugin};
    const std::string departureRunway = "22L";
    const std::string airport = "EKCH";
    const std::string expected =
        "EKCH/22L SALLO M44 KOGIM M602 USEDU DCT LUROS M725 HDO DCT EVIKU DCT BITSI DCT MIKOV MIKOV8W";
};

TEST_F(SetDepartureRunwayTest, WithSpecifiedAndBaseSid) {
    std::string route =
        "SALLO1C SALLO M44 KOGIM M602 USEDU DCT LUROS M725 HDO DCT EVIKU DCT BITSI DCT MIKOV MIKOV8W";
    service.SetDepartureRunway(route, departureRunway, airport);
    EXPECT_EQ(route, expected);
}

TEST_F(SetDepartureRunwayTest, WithOnlySpecifiedSid) {
    std::string route =
        "SALLO1C M44 KOGIM M602 USEDU DCT LUROS M725 HDO DCT EVIKU DCT BITSI DCT MIKOV MIKOV8W";
    service.SetDepartureRunway(route, departureRunway, airport);
    EXPECT_EQ(route, expected);
}

TEST_F(SetDepartureRunwayTest, WithOnlyBaseSid) {
    std::string route =
        "SALLO M44 KOGIM M602 USEDU DCT LUROS M725 HDO DCT EVIKU DCT BITSI DCT MIKOV MIKOV8W";
    service.SetDepartureRunway(route, departureRunway, airport);
    EXPECT_EQ(route, expected);
}

TEST_F(SetDepartureRunwayTest, WithRunwayAndBaseSid) {
    std::string route =
        "EKCH/12 SALLO M44 KOGIM M602 USEDU DCT LUROS M725 HDO DCT EVIKU DCT BITSI DCT MIKOV MIKOV8W";
    service.SetDepartureRunway(route, departureRunway, airport);
    EXPECT_EQ(route, expected);
}

TEST_F(SetDepartureRunwayTest, WithRunwayAndSpecifiedSid) {
    std::string route =
        "EKCH/22R SALLO1C SALLO M44 KOGIM M602 USEDU DCT LUROS M725 HDO DCT EVIKU DCT BITSI DCT MIKOV MIKOV8W";
    service.SetDepartureRunway(route, departureRunway, airport);
    EXPECT_EQ(route, expected);
}

TEST_F(SetDepartureRunwayTest, WithRunwayAndSpecifiedSidLowerCase) {
    std::string route =
        "ekch/22r sallo1c sallo m44 kogim m602 usedu dct luros m725 hdo dct eviku dct bitsi dct mikov mikov8w";
    service.SetDepartureRunway(route, departureRunway, airport);
    EXPECT_EQ(route, expected);
}

TEST_F(SetDepartureRunwayTest, AlreadyCorrectRunwayDoesNothing) {
    std::string route =
        "EKCH/22L SALLO1F SALLO M44 KOGIM M602 USEDU DCT LUROS M725 HDO DCT EVIKU DCT BITSI DCT MIKOV MIKOV8W";
    const std::string original = route;
    service.SetDepartureRunway(route, departureRunway, airport);
    EXPECT_EQ(route, original);
}

TEST_F(SetDepartureRunwayTest, EmptyRoute_SetsJustRunwayToken) {
    // With an empty route the runway token should become the entire route.
    std::string route;
    service.SetDepartureRunway(route, departureRunway, airport);
    EXPECT_EQ(route, "EKCH/22L");
}
