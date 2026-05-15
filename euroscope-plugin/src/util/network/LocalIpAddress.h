#pragma once

#include <optional>

namespace FlightStrips::network {
    struct LocalAddressCandidate {
        std::string address;
        std::string friendlyName;
        std::string description;
        unsigned long interfaceIndex = 0;
        unsigned long metric = 0;
        unsigned long interfaceType = 0;
        unsigned long operStatus = 0;
    };

    [[nodiscard]] bool IsPrivateIPv4(const std::string& address);
    [[nodiscard]] std::optional<std::string> SelectPreferredPrivateIPv4(
        const std::vector<LocalAddressCandidate>& candidates,
        std::optional<unsigned long> preferredInterfaceIndex = std::nullopt);
    [[nodiscard]] std::optional<std::string> GetLocalPrivateIPv4();
}
