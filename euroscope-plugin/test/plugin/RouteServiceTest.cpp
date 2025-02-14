#include <memory>
#include <string>
#include <vector>

#include "doctest.h"
#include "flightplan/RouteService.h"


using FlightStrips::flightplan::RouteService;


class FakePlugin final : public FlightStrips::FlightStripsPluginInterface {
public:
    std::vector<FlightStrips::Sid> GetSids(const std::string &airport) override {
        return {
            FlightStrips::Sid("LANGO2C", "22R"), FlightStrips::Sid("NEXEN2C", "22R"),
            FlightStrips::Sid("SALLO1C", "22R")
        };
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
        SUBCASE("With airport") {
            route = "EKCH/22R NEXEN2C T503 GIMRU DCT MICOS DCT RIMET/N0495F390 T157 KERAX KERAX3A";
        }
        SUBCASE("With airport lower case") {
            route = "ekch/22r nexen2c t503 gimru dct micos dct rimet/n0495f390 t157 kerax kerax3a";
        }
        SUBCASE("NO SID") { route = "T503 GIMRU DCT MICOS DCT RIMET/N0495F390 T157 KERAX KERAX3A"; }
        CAPTURE(route);

        service.SetSid(route, newSid, airport);

        CHECK(route == "LANGO2C T503 GIMRU DCT MICOS DCT RIMET/N0495F390 T157 KERAX KERAX3A");
    }

    TEST_CASE("Already correct SID does nothing") {
        const auto plugin = std::make_shared<FakePlugin>();
        const auto service = RouteService(plugin);
        std::string route = "LANGO2C LANGO T503 GIMRU DCT MICOS DCT RIMET/N0495F390 T157 KERAX KERAX3A";
        std::string original = std::string(route);
        const std::string newSid = "LANGO2C";
        const std::string airport = "EKCH";

        service.SetSid(route, newSid, airport);

        CHECK(route == original);
    }
}

TEST_SUITE("SetDepartureRunway") {
    TEST_CASE("Sets correct runway") {
        const auto plugin = std::make_shared<FakePlugin>();
        const auto service = RouteService(plugin);
        std::string route;
        const std::string departureRunway = "22L";
        const std::string airport = "EKCH";

        SUBCASE("With specified and base SID") {
            route = "SALLO1C SALLO M44 KOGIM M602 USEDU DCT LUROS M725 HDO DCT EVIKU DCT BITSI DCT MIKOV MIKOV8W";
        }
        SUBCASE("With only specified SID") {
            route = "SALLO1C M44 KOGIM M602 USEDU DCT LUROS M725 HDO DCT EVIKU DCT BITSI DCT MIKOV MIKOV8W";
        }
        SUBCASE("With only base SID") {
            route = "SALLO M44 KOGIM M602 USEDU DCT LUROS M725 HDO DCT EVIKU DCT BITSI DCT MIKOV MIKOV8W";
        }
        SUBCASE("With runway and base SID") {
            route = "EKCH/12 SALLO M44 KOGIM M602 USEDU DCT LUROS M725 HDO DCT EVIKU DCT BITSI DCT MIKOV MIKOV8W";
        }
        SUBCASE("With runway and specified SID") {
            route =
                    "EKCH/22R SALLO1C SALLO M44 KOGIM M602 USEDU DCT LUROS M725 HDO DCT EVIKU DCT BITSI DCT MIKOV MIKOV8W";
        }
        SUBCASE("With runway and specified SID lower case") {
            route =
                    "ekch/22r sallo1c sallo m44 kogim m602 usedu dct luros m725 hdo dct eviku dct bitsi dct mikov mikov8w";
        }
        CAPTURE(route);

        service.SetDepartureRunway(route, departureRunway, airport);

        CHECK(route == "EKCH/22L SALLO M44 KOGIM M602 USEDU DCT LUROS M725 HDO DCT EVIKU DCT BITSI DCT MIKOV MIKOV8W");
    }

    TEST_CASE("Already correct departure runway does nothing") {
        const auto plugin = std::make_shared<FakePlugin>();
        const auto service = RouteService(plugin);
        std::string route =
                "EKCH/22L SALLO1F SALLO M44 KOGIM M602 USEDU DCT LUROS M725 HDO DCT EVIKU DCT BITSI DCT MIKOV MIKOV8W";
        std::string original = std::string(route);
        const std::string departureRunway = "22L";
        const std::string airport = "EKCH";

        service.SetDepartureRunway(route, departureRunway, airport);

        CHECK(route == original);
    }
}
