#include <gtest/gtest.h>

#include "graphics/InfoPanel.h"

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
