#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "configuration/AppConfig.h"

using FlightStrips::configuration::AppConfig;

// All tests use a nonexistent path so Config opens with an empty INI,
// giving us pure default values without touching the filesystem.

static AppConfig MakeDefault() {
    return AppConfig("__nonexistent_app_config__.ini");
}

// ---------------------------------------------------------------------------
// Default values
// ---------------------------------------------------------------------------

TEST(AppConfigTest, GetAuthority_Default_ReturnsError) {
    auto cfg = MakeDefault();
    EXPECT_EQ(cfg.GetAuthority(), "error");
}

TEST(AppConfigTest, GetAudience_Default_ReturnsError) {
    auto cfg = MakeDefault();
    EXPECT_EQ(cfg.GetAudience(), "error");
}

TEST(AppConfigTest, GetClientId_Default_ReturnsError) {
    auto cfg = MakeDefault();
    EXPECT_EQ(cfg.GetClientId(), "error");
}

TEST(AppConfigTest, GetScopes_Default_ReturnsOpenidProfile) {
    auto cfg = MakeDefault();
    EXPECT_EQ(cfg.GetScopes(), "openid profile offline_access");
}

TEST(AppConfigTest, GetRedirectPort_Default_Returns27015) {
    auto cfg = MakeDefault();
    EXPECT_EQ(cfg.GetRedirectPort(), 27015);
}

TEST(AppConfigTest, GetBaseUrl_Default_ReturnsError) {
    auto cfg = MakeDefault();
    EXPECT_EQ(cfg.GetBaseUrl(), "error");
}

TEST(AppConfigTest, GetApiEnabled_Default_ReturnsFalse) {
    auto cfg = MakeDefault();
    EXPECT_FALSE(cfg.GetApiEnabled());
}

TEST(AppConfigTest, GetLogLevel_Default_ReturnsInfo) {
    auto cfg = MakeDefault();
    EXPECT_EQ(cfg.GetLogLevel(), "INFO");
}

TEST(AppConfigTest, GetPositionUpdateIntervalSeconds_Default_Returns10) {
    auto cfg = MakeDefault();
    EXPECT_EQ(cfg.GetPositionUpdateIntervalSeconds(), 10);
}

TEST(AppConfigTest, GetStandsFile_Default_ReturnsGRpluginStands) {
    auto cfg = MakeDefault();
    EXPECT_EQ(cfg.GetStandsFile(), "GRpluginStands.txt");
}

TEST(AppConfigTest, GetDisconnectOnOutOfRange_Default_ReturnsFalse) {
    auto cfg = MakeDefault();
    EXPECT_FALSE(cfg.GetDisconnectOnOutOfRange());
}

// ---------------------------------------------------------------------------
// GetCallsignAirportMap — when empty, falls back to EKCH
// ---------------------------------------------------------------------------

TEST(AppConfigTest, GetCallsignAirportMap_Default_ContainsEkch) {
    auto cfg = MakeDefault();
    auto& map = cfg.GetCallsignAirportMap();
    EXPECT_FALSE(map.empty());
    EXPECT_NE(map.find("EKCH"), map.end());
}

TEST(AppConfigTest, GetCallsignAirportMap_SecondCall_ReturnsSameReference) {
    // GetCallsignAirportMap caches its result; two calls must return the
    // same memory address (same object).
    auto cfg = MakeDefault();
    auto& first  = cfg.GetCallsignAirportMap();
    auto& second = cfg.GetCallsignAirportMap();
    EXPECT_EQ(&first, &second);
}

TEST(AppConfigTest, GetCallsignAirportMap_EkchEntry_HasAtLeastOnePrefix) {
    auto cfg = MakeDefault();
    auto& map = cfg.GetCallsignAirportMap();
    const auto it = map.find("EKCH");
    ASSERT_NE(it, map.end());
    EXPECT_FALSE(it->second.empty());
}

// ---------------------------------------------------------------------------
// GetDeIceConfig — empty INI yields empty DeIceConfig
// ---------------------------------------------------------------------------

TEST(AppConfigTest, GetDeIceConfig_Default_OrderIsEmpty) {
    auto cfg = MakeDefault();
    const auto& deice = cfg.GetDeIceConfig();
    EXPECT_TRUE(deice.order.empty());
}

TEST(AppConfigTest, GetDeIceConfig_Default_AcTypesIsEmpty) {
    auto cfg = MakeDefault();
    const auto& deice = cfg.GetDeIceConfig();
    EXPECT_TRUE(deice.ac_types.empty());
}

TEST(AppConfigTest, GetDeIceConfig_Default_AirlinesIsEmpty) {
    auto cfg = MakeDefault();
    const auto& deice = cfg.GetDeIceConfig();
    EXPECT_TRUE(deice.airlines.empty());
}

TEST(AppConfigTest, GetDeIceConfig_Default_StandsIsEmpty) {
    auto cfg = MakeDefault();
    const auto& deice = cfg.GetDeIceConfig();
    EXPECT_TRUE(deice.stands.empty());
}

TEST(AppConfigTest, GetDeIceConfig_Default_FallbackIsEmpty) {
    auto cfg = MakeDefault();
    const auto& deice = cfg.GetDeIceConfig();
    EXPECT_TRUE(deice.fallback.empty());
}

TEST(AppConfigTest, GetDeIceConfig_SecondCall_ReturnsSameReference) {
    // GetDeIceConfig also caches; second call must be same object.
    auto cfg = MakeDefault();
    const auto& first  = cfg.GetDeIceConfig();
    const auto& second = cfg.GetDeIceConfig();
    EXPECT_EQ(&first, &second);
}
