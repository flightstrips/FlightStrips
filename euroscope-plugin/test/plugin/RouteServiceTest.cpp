
#include <memory>
#include <string>
#include <vector>

#include "doctest.h"
#include "flightplan/RouteService.h"


using FlightStrips::flightplan::RouteService;


class FakePlugin final : public FlightStrips::FlightStripsPluginInterface {

public:
    std::vector<FlightStrips::Sid> GetSids(const std::string& airport) override {
        return {FlightStrips::Sid("LANGO2C", "22R") , FlightStrips::Sid("NEXEN2C", "22R")};
    };
};

TEST_SUITE("SetSid") {

    TEST_CASE("Replaces existing SID") {
        const auto plugin = std::make_shared<FakePlugin>();
        const auto service = RouteService(plugin);
        std::string route;
        const std::string newSid = "LANGO2C";
        const std::string airport = "EKCH";

        SUBCASE("Base SID") { route = "NEXEN T503 GIMRU DCT MICOS DCT RIMET/N0495F390 T157 KERAX KERAX3A"; }
        SUBCASE("Full SID") { route = "NEXEN2C T503 GIMRU DCT MICOS DCT RIMET/N0495F390 T157 KERAX KERAX3A"; }
        SUBCASE("With airport") { route = "EKCH/22R NEXEN2C T503 GIMRU DCT MICOS DCT RIMET/N0495F390 T157 KERAX KERAX3A"; }
        SUBCASE("NO SID") { route = "T503 GIMRU DCT MICOS DCT RIMET/N0495F390 T157 KERAX KERAX3A"; }
        CAPTURE(route);

        service.SetSid(route, newSid, airport);

        CHECK(route == "LANGO2C T503 GIMRU DCT MICOS DCT RIMET/N0495F390 T157 KERAX KERAX3A");
    }
}