#include <gtest/gtest.h>

#include "graphics/InfoPanel.h"

using FlightStrips::authentication::AUTHENTICATED;
using FlightStrips::authentication::NONE;
using FlightStrips::graphics::AuthenticationButtonLayout;
using FlightStrips::graphics::CalculateAuthenticationButtonLayout;
using FlightStrips::ConnectionState;
using FlightStrips::graphics::CalculateInfoPanelContentHeight;
using FlightStrips::graphics::InfoPanelData;

TEST(InfoPanelTest, CalculateContentHeightWithoutStatsUsesCollapsedHeight) {
    const InfoPanelData data{
        .connectionState = ConnectionState{},
        .sessionSelectable = false,
        .showStats = false,
    };

    EXPECT_EQ(CalculateInfoPanelContentHeight(data), 129);
}

TEST(InfoPanelTest, CalculateContentHeightWithConnectedStatsAddsFullStatsBlock) {
    const InfoPanelData data{
        .connectionState = ConnectionState{},
        .sessionSelectable = true,
        .connected = true,
        .showStats = true,
    };

    EXPECT_EQ(CalculateInfoPanelContentHeight(data), 196);
}

TEST(InfoPanelTest, CalculateContentHeightWithDisconnectedStatsOmitsConnectionRows) {
    const InfoPanelData data{
        .connectionState = ConnectionState{},
        .sessionSelectable = true,
        .connected = false,
        .showStats = true,
    };

    EXPECT_EQ(CalculateInfoPanelContentHeight(data), 157);
}

TEST(InfoPanelTest, CalculateAuthenticationButtonLayoutShowsOpenAppWhenAuthenticated) {
    const InfoPanelData data{
        .authenticationState = AUTHENTICATED,
    };

    const AuthenticationButtonLayout layout = CalculateAuthenticationButtonLayout(data, 10, 160, 20);

    EXPECT_EQ(layout.authenticationButtonRect.left, 15);
    EXPECT_EQ(layout.authenticationButtonRect.top, 58);
    EXPECT_EQ(layout.authenticationButtonRect.right, 83);
    EXPECT_EQ(layout.authenticationButtonRect.bottom, 74);
    ASSERT_TRUE(layout.openAppButtonRect.has_value());
    EXPECT_EQ(layout.openAppButtonRect->left, 87);
    EXPECT_EQ(layout.openAppButtonRect->top, 58);
    EXPECT_EQ(layout.openAppButtonRect->right, 155);
    EXPECT_EQ(layout.openAppButtonRect->bottom, 74);
}

TEST(InfoPanelTest, CalculateAuthenticationButtonLayoutHidesOpenAppWhenLoggedOut) {
    const InfoPanelData data{
        .authenticationState = NONE,
    };

    const AuthenticationButtonLayout layout = CalculateAuthenticationButtonLayout(data, 10, 160, 20);

    EXPECT_EQ(layout.authenticationButtonRect.left, 15);
    EXPECT_EQ(layout.authenticationButtonRect.top, 58);
    EXPECT_EQ(layout.authenticationButtonRect.right, 90);
    EXPECT_EQ(layout.authenticationButtonRect.bottom, 74);
    EXPECT_FALSE(layout.openAppButtonRect.has_value());
}
