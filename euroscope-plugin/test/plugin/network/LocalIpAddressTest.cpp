#include <gtest/gtest.h>

#include "network/LocalIpAddress.h"

using FlightStrips::network::LocalAddressCandidate;
using FlightStrips::network::SelectPreferredPrivateIPv4;

TEST(LocalIpAddressTest, SelectPreferredPrivateIPv4_IgnoresVirtualAdapters) {
    const std::vector<LocalAddressCandidate> candidates = {
        {
            .address = "172.20.64.1",
            .friendlyName = "vEthernet (WSL)",
            .description = "Hyper-V Virtual Ethernet Adapter",
            .interfaceIndex = 17,
            .metric = 5,
            .interfaceType = 6,
            .operStatus = 1,
        },
        {
            .address = "192.168.1.25",
            .friendlyName = "Wi-Fi",
            .description = "Intel Wireless Adapter",
            .interfaceIndex = 9,
            .metric = 35,
            .interfaceType = 71,
            .operStatus = 1,
        },
    };

    const auto selected = SelectPreferredPrivateIPv4(candidates, 17);
    ASSERT_TRUE(selected.has_value());
    EXPECT_EQ(selected.value(), "192.168.1.25");
}

TEST(LocalIpAddressTest, SelectPreferredPrivateIPv4_PrefersBestLanInterfaceWhenAvailable) {
    const std::vector<LocalAddressCandidate> candidates = {
        {
            .address = "192.168.0.20",
            .friendlyName = "Ethernet",
            .description = "Intel Ethernet Controller",
            .interfaceIndex = 3,
            .metric = 25,
            .interfaceType = 6,
            .operStatus = 1,
        },
        {
            .address = "192.168.0.42",
            .friendlyName = "Wi-Fi",
            .description = "Intel Wireless Adapter",
            .interfaceIndex = 7,
            .metric = 35,
            .interfaceType = 71,
            .operStatus = 1,
        },
    };

    const auto selected = SelectPreferredPrivateIPv4(candidates, 7);
    ASSERT_TRUE(selected.has_value());
    EXPECT_EQ(selected.value(), "192.168.0.42");
}

TEST(LocalIpAddressTest, SelectPreferredPrivateIPv4_ReturnsEmptyWhenNoUsableLanAddressExists) {
    const std::vector<LocalAddressCandidate> candidates = {
        {
            .address = "169.254.10.20",
            .friendlyName = "Ethernet",
            .description = "Intel Ethernet Controller",
            .interfaceIndex = 3,
            .metric = 5,
            .interfaceType = 6,
            .operStatus = 1,
        },
        {
            .address = "172.20.64.1",
            .friendlyName = "vEthernet (WSL)",
            .description = "Hyper-V Virtual Ethernet Adapter",
            .interfaceIndex = 17,
            .metric = 5,
            .interfaceType = 6,
            .operStatus = 1,
        },
    };

    EXPECT_FALSE(SelectPreferredPrivateIPv4(candidates, 3).has_value());
}
