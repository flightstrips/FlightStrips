
#include "doctest.h"
#include "authentication/AuthenticationService.h"

using FlightStrips::authentication::AuthenticationService;

TEST_SUITE("Token parsing") {
    TEST_CASE("Empty token") {
        const auto result = AuthenticationService::GetTokenPayload("");
        CHECK(result.has_value() == false);
    }


    TEST_CASE("Invalid token") {
        const auto result = AuthenticationService::GetTokenPayload("This is an invalid token");
        CHECK(result.has_value() == false);
    }

    TEST_CASE("Valid token") {
        const auto token = "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IkV3NlhiWEoxTHN6UWtwY2FxeE1OdiJ9.eyJpc3MiOiJodHRwczovL2Rldi14ZDB1ZjRzZDF2MjdyOHRnLmV1LmF1dGgwLmNvbS8iLCJzdWIiOiJvYXV0aDJ8dmF0c2ltLWRldnwxMDAwMDAwNSIsImF1ZCI6WyJiYWNrZW5kIiwiaHR0cHM6Ly9kZXYteGQwdWY0c2QxdjI3cjh0Zy5ldS5hdXRoMC5jb20vdXNlcmluZm8iXSwiaWF0IjoxNzM4NjE1NzA4LCJleHAiOjE3Mzg3MDIxMDgsInNjb3BlIjoib3BlbmlkIHByb2ZpbGUgb2ZmbGluZV9hY2Nlc3MiLCJhenAiOiJsTWZxQkRraURrUG5jZ3FCOWxMWFNqOTB3cjUxejNDaSJ9.SOQVdyVG0Ok2ytPvbHFu0uWlG8d75BxtKA82iek9mq0H0yFgK2T-JXZINdSissGSjAlFAejuG3IVhkRFIiOSzaval6ajXO4750nhmqurZrCccW1k8-lUiknNcPcOsLwvg83XnSYJgLAGQxqVNPsfP9Xf76GdN3fxQ-zPiErOy0Y-lKYrzaMoRWRYp_CiMEvAAIn--sFruvme0yuZfv4XDeH9sMtKTJ-iQ70lM0U6oPcxUEr444BIBUEriqwGwdUhZbnno01MpVwAabMP4A-4pXFRxUvy9CkkVdjl1xxDRyjBD22v2SizPWMuB7dsBvwgDD9I7kHB6MUMb6ysVimDsA";
        const auto result = AuthenticationService::GetTokenPayload(token);

        REQUIRE(result.has_value() == true);
        const auto& json = result.value();

        CHECK(json["iss"] == "https://dev-xd0uf4sd1v27r8tg.eu.auth0.com/");
        CHECK(json["sub"] == "oauth2|vatsim-dev|10000005");
        CHECK(json["iat"] == 1738615708);
        CHECK(json["exp"] == 1738702108);
        CHECK(json["scope"] == "openid profile offline_access");
    }
}
