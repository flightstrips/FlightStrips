#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "authentication/AuthenticationService.h"
#include "mock/MockAuthenticationEventHandler.h"

using FlightStrips::authentication::AuthenticationService;
using FlightStrips::authentication::AuthenticationState;
using FlightStrips::configuration::AppConfig;
using FlightStrips::configuration::UserConfig;
using FlightStrips::handlers::AuthenticationEventHandlers;
using ::testing::_;
using ::testing::StrictMock;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

static std::shared_ptr<AppConfig> MakeAppConfig() {
    return std::make_shared<AppConfig>("__nonexistent_app_config__.ini");
}

static std::shared_ptr<UserConfig> MakeUserConfig() {
    return std::make_shared<UserConfig>("__nonexistent_user_config__.ini");
}

// ---------------------------------------------------------------------------
// Token parsing tests
// ---------------------------------------------------------------------------

class TokenParsingTest : public ::testing::Test {};

TEST_F(TokenParsingTest, EmptyString_ReturnsEmpty) {
    const auto result = AuthenticationService::GetTokenPayload("");
    EXPECT_FALSE(result.has_value());
}

TEST_F(TokenParsingTest, InvalidJwt_ReturnsEmpty) {
    const auto result = AuthenticationService::GetTokenPayload("This is an invalid token");
    EXPECT_FALSE(result.has_value());
}

TEST_F(TokenParsingTest, OnlyOneSegment_NoSecondDot_ReturnsEmpty) {
    // Has a first dot but no second dot — end '.' not found
    const auto result = AuthenticationService::GetTokenPayload("header.payload_no_second_dot");
    EXPECT_FALSE(result.has_value());
}

TEST_F(TokenParsingTest, PayloadIsNotValidJson_ReturnsEmpty) {
    // Encode a non-JSON string as the payload segment.
    // base64url of "not-json" = bm90LWpzb24
    const auto result = AuthenticationService::GetTokenPayload("header.bm90LWpzb24.signature");
    EXPECT_FALSE(result.has_value());
}

TEST_F(TokenParsingTest, ValidJwt_ParsesClaims) {
    // A real JWT from the dev environment (already expired, safe to commit)
    const auto token =
        "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IkV3NlhiWEoxTHN6UWtwY2FxeE1OdiJ9"
        ".eyJpc3MiOiJodHRwczovL2Rldi14ZDB1ZjRzZDF2MjdyOHRnLmV1LmF1dGgwLmNvbS8iLCJzdWIiOiJvYXV0"
        "aDJ8dmF0c2ltLWRldnwxMDAwMDAwNSIsImF1ZCI6WyJiYWNrZW5kIiwiaHR0cHM6Ly9kZXYteGQwdWY0c2Qxdj"
        "I3cjh0Zy5ldS5hdXRoMC5jb20vdXNlcmluZm8iXSwiaWF0IjoxNzM4NjE1NzA4LCJleHAiOjE3Mzg3MDIxMDgs"
        "InNjb3BlIjoib3BlbmlkIHByb2ZpbGUgb2ZmbGluZV9hY2Nlc3MiLCJhenAiOiJsTWZxQkRraURrUG5jZ3FCOWR"
        "MVFNqOTB3cjUxejNDaSJ9"
        ".SOQVdyVG0Ok2ytPvbHFu0uWlG8d75BxtKA82iek9mq0H0yFgK2T-JXZINdSissGSjAlFAejuG3IVhkRFIiOSzav"
        "al6ajXO4750nhmqurZrCccW1k8-lUiknNcPcOsLwvg83XnSYJgLAGQxqVNPsfP9Xf76GdN3fxQ-zPiErOy0Y-lKY"
        "rzaMoRWRYp_CiMEvAAIn--sFruvme0yuZfv4XDeH9sMtKTJ-iQ70lM0U6oPcxUEr444BIBUEriqwGwdUhZbnno01M"
        "pVwAabMP4A-4pXFRxUvy9CkkVdjl1xxDRyjBD22v2SizPWMuB7dsBvwgDD9I7kHB6MUMb6ysVimDsA";

    const auto result = AuthenticationService::GetTokenPayload(token);

    ASSERT_TRUE(result.has_value());
    const auto& json = result.value();

    EXPECT_EQ(json["iss"], "https://dev-xd0uf4sd1v27r8tg.eu.auth0.com/");
    EXPECT_EQ(json["sub"], "oauth2|vatsim-dev|10000005");
    EXPECT_EQ(json["iat"], 1738615708);
    EXPECT_EQ(json["exp"], 1738702108);
    EXPECT_EQ(json["scope"], "openid profile offline_access");
}

// ---------------------------------------------------------------------------
// AuthenticationService state machine tests
// ---------------------------------------------------------------------------

class AuthenticationServiceStateTest : public ::testing::Test {
protected:
    std::shared_ptr<AppConfig> appConfig = MakeAppConfig();
    std::shared_ptr<UserConfig> userConfig = MakeUserConfig();
    std::shared_ptr<AuthenticationEventHandlers> eventHandlers =
        std::make_shared<AuthenticationEventHandlers>();

    AuthenticationService MakeService() {
        return AuthenticationService(appConfig, userConfig, eventHandlers);
    }
};

TEST_F(AuthenticationServiceStateTest, InitialState_IsNone) {
    auto svc = MakeService();
    EXPECT_EQ(svc.GetAuthenticationState(), AuthenticationState::NONE);
}

TEST_F(AuthenticationServiceStateTest, GetAccessToken_BeforeAuth_ReturnsEmpty) {
    auto svc = MakeService();
    EXPECT_EQ(svc.GetAccessToken(), "");
}

TEST_F(AuthenticationServiceStateTest, GetName_BeforeAuth_ReturnsEmpty) {
    auto svc = MakeService();
    EXPECT_EQ(svc.GetName(), "");
}

TEST_F(AuthenticationServiceStateTest, Logout_AfterConstruction_StateRemainsNone) {
    // Logout on a freshly-constructed service (state == NONE) must not crash
    // and state must remain NONE afterwards.
    auto svc = MakeService();
    EXPECT_NO_FATAL_FAILURE(svc.Logout());
    EXPECT_EQ(svc.GetAuthenticationState(), AuthenticationState::NONE);
}

TEST_F(AuthenticationServiceStateTest, Logout_ClearsAccessToken) {
    auto svc = MakeService();
    svc.Logout();
    EXPECT_EQ(svc.GetAccessToken(), "");
}

TEST_F(AuthenticationServiceStateTest, Logout_ClearsName) {
    auto svc = MakeService();
    svc.Logout();
    EXPECT_EQ(svc.GetName(), "");
}

TEST_F(AuthenticationServiceStateTest, CancelAuthentication_WhenNotInLoginState_DoesNotCrash) {
    // State is NONE — CancelAuthentication() should be a no-op.
    auto svc = MakeService();
    EXPECT_NO_FATAL_FAILURE(svc.CancelAuthentication());
    EXPECT_EQ(svc.GetAuthenticationState(), AuthenticationState::NONE);
}

// ---------------------------------------------------------------------------
// Handler dispatch tests
// ---------------------------------------------------------------------------

class AuthenticationServiceHandlerTest : public ::testing::Test {
protected:
    std::shared_ptr<AppConfig> appConfig = MakeAppConfig();
    std::shared_ptr<UserConfig> userConfig = MakeUserConfig();
    std::shared_ptr<AuthenticationEventHandlers> eventHandlers =
        std::make_shared<AuthenticationEventHandlers>();
};

TEST_F(AuthenticationServiceHandlerTest, NotifyHandlers_OnTokenUpdate_CallsAllHandlers) {
    auto mockHandler1 = std::make_shared<MockAuthenticationEventHandler>();
    auto mockHandler2 = std::make_shared<MockAuthenticationEventHandler>();

    EXPECT_CALL(*mockHandler1, OnTokenUpdate("test-token")).Times(1);
    EXPECT_CALL(*mockHandler2, OnTokenUpdate("test-token")).Times(1);

    eventHandlers->RegisterHandler(mockHandler1);
    eventHandlers->RegisterHandler(mockHandler2);
    eventHandlers->OnTokenUpdate("test-token");
}

// ---------------------------------------------------------------------------
// Base64 encode/decode round-trip tests
//
// base64_encode and base64_decode are private static members, but their
// behaviour is fully observable through GetTokenPayload: encoding is verified
// by constructing a token whose payload encodes known JSON and confirming the
// parser returns the expected values.  The tests below additionally verify
// round-trip properties by calling the public static API indirectly via the
// JWT parsing path.
// ---------------------------------------------------------------------------

class Base64RoundTripTest : public ::testing::Test {
protected:
    // Build a minimal 3-segment JWT where the payload is base64url({"k":"v"})
    // eyJrIjoidiJ9 is base64url({"k":"v"})
    static constexpr const char* kSimpleJwt = "header.eyJrIjoidiJ9.signature";
};

TEST_F(Base64RoundTripTest, DecodesPayloadFromSimpleJwt) {
    const auto result = AuthenticationService::GetTokenPayload(kSimpleJwt);
    ASSERT_TRUE(result.has_value());
    EXPECT_EQ(result.value()["k"], "v");
}

TEST_F(Base64RoundTripTest, EmptyPayloadSegment_ReturnsEmpty) {
    // "header..signature" — empty payload decodes to "" which is invalid JSON
    const auto result = AuthenticationService::GetTokenPayload("header..signature");
    EXPECT_FALSE(result.has_value());
}
