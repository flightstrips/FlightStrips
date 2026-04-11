#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "configuration/UserConfig.h"

using FlightStrips::configuration::UserConfig;
using FlightStrips::configuration::Token;
using FlightStrips::configuration::WindowState;

// All tests use a nonexistent path so Config opens with an empty INI,
// giving us pure default values without touching the filesystem.

static UserConfig MakeDefault() {
    return UserConfig("__nonexistent_user_config__.ini");
}

// ---------------------------------------------------------------------------
// GetToken defaults
// ---------------------------------------------------------------------------

TEST(UserConfigTest, GetToken_Default_AccessTokenEmpty) {
    auto cfg = MakeDefault();
    EXPECT_EQ(cfg.GetToken().accessToken, "");
}

TEST(UserConfigTest, GetToken_Default_RefreshTokenEmpty) {
    auto cfg = MakeDefault();
    EXPECT_EQ(cfg.GetToken().refreshToken, "");
}

TEST(UserConfigTest, GetToken_Default_IdTokenEmpty) {
    auto cfg = MakeDefault();
    EXPECT_EQ(cfg.GetToken().idToken, "");
}

TEST(UserConfigTest, GetToken_Default_ExpiryIsZero) {
    auto cfg = MakeDefault();
    EXPECT_EQ(cfg.GetToken().expiry, 0);
}

// ---------------------------------------------------------------------------
// SetToken / GetToken round-trip
//
// SetToken writes to the INI and calls save().  When the path is
// "__nonexistent_user_config__.ini", save() opens a new file, writes it, and
// leaves it on disk.  We therefore use a path in the system temp directory
// with a unique name so tests don't interfere with each other and so we
// always clean up.
// ---------------------------------------------------------------------------

class UserConfigRoundTripTest : public ::testing::Test {
protected:
    std::string tmpPath;

    void SetUp() override {
        // Build a temp path that is unlikely to collide.
        char buf[MAX_PATH];
        GetTempPathA(MAX_PATH, buf);
        tmpPath = std::string(buf) + "userconfig_test_" +
                  std::to_string(GetCurrentProcessId()) + "_" +
                  std::to_string(GetTickCount64()) + ".ini";
    }

    void TearDown() override {
        // Remove temp file even if the test fails.
        DeleteFileA(tmpPath.c_str());
    }
};

TEST_F(UserConfigRoundTripTest, SetToken_ThenGetToken_RoundTrips) {
    Token t;
    t.accessToken  = "my-access-token";
    t.refreshToken = "my-refresh-token";
    t.idToken      = "my-id-token";
    t.expiry       = 1700000000;

    {
        UserConfig cfg(tmpPath);
        cfg.SetToken(t);
    }

    // Re-open from the written file.
    UserConfig cfg2(tmpPath);
    const auto got = cfg2.GetToken();
    EXPECT_EQ(got.accessToken,  t.accessToken);
    EXPECT_EQ(got.refreshToken, t.refreshToken);
    EXPECT_EQ(got.idToken,      t.idToken);
    EXPECT_EQ(got.expiry,       t.expiry);
}

TEST_F(UserConfigRoundTripTest, SetToken_EmptyToken_ClearsFields) {
    // First write a real token, then overwrite with an empty one.
    {
        UserConfig cfg(tmpPath);
        cfg.SetToken({"a", "b", "c", 999});
        cfg.SetToken({});  // clear
    }

    UserConfig cfg2(tmpPath);
    const auto got = cfg2.GetToken();
    EXPECT_EQ(got.accessToken,  "");
    EXPECT_EQ(got.refreshToken, "");
    EXPECT_EQ(got.idToken,      "");
    EXPECT_EQ(got.expiry,       0);
}

TEST_F(UserConfigRoundTripTest, SetWindowState_ThenGetWindowState_RoundTrips) {
    WindowState ws{123, 456, true};

    {
        UserConfig cfg(tmpPath);
        cfg.SetWindowState(ws);
    }

    UserConfig cfg2(tmpPath);
    const auto got = cfg2.GetWindowState();
    EXPECT_EQ(got.x,         ws.x);
    EXPECT_EQ(got.y,         ws.y);
    EXPECT_EQ(got.minimized, ws.minimized);
}

// ---------------------------------------------------------------------------
// GetWindowState defaults
// ---------------------------------------------------------------------------

TEST(UserConfigTest, GetWindowState_Default_XIs400) {
    auto cfg = MakeDefault();
    EXPECT_EQ(cfg.GetWindowState().x, 400);
}

TEST(UserConfigTest, GetWindowState_Default_YIs400) {
    auto cfg = MakeDefault();
    EXPECT_EQ(cfg.GetWindowState().y, 400);
}

TEST(UserConfigTest, GetWindowState_Default_NotMinimized) {
    auto cfg = MakeDefault();
    EXPECT_FALSE(cfg.GetWindowState().minimized);
}

TEST(UserConfigTest, GetPreferSweatboxSession_DefaultsFalse) {
    auto cfg = MakeDefault();
    EXPECT_FALSE(cfg.GetPreferSweatboxSession());
}

TEST_F(UserConfigRoundTripTest, SetPreferSweatboxSession_ThenGetPreferSweatboxSession_RoundTrips) {
    {
        UserConfig cfg(tmpPath);
        cfg.SetPreferSweatboxSession(true);
    }

    UserConfig cfg2(tmpPath);
    EXPECT_TRUE(cfg2.GetPreferSweatboxSession());
}

TEST_F(UserConfigRoundTripTest, SetPreferSweatboxSession_False_RoundTrips) {
    {
        UserConfig cfg(tmpPath);
        cfg.SetPreferSweatboxSession(true);
        cfg.SetPreferSweatboxSession(false);
    }

    UserConfig cfg2(tmpPath);
    EXPECT_FALSE(cfg2.GetPreferSweatboxSession());
}
