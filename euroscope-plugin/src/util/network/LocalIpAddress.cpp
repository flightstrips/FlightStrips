#include "network/LocalIpAddress.h"

#include <Iphlpapi.h>
#include <ipifcons.h>
#include <cstddef>

namespace FlightStrips::network {
    namespace {
        std::optional<std::array<int, 4> > ParseIPv4Octets(const std::string& address) {
            std::array<int, 4> octets{};
            std::stringstream stream(address);
            std::string part;

            for (auto i = 0; i < 4; ++i) {
                if (!std::getline(stream, part, '.')) {
                    return std::nullopt;
                }

                if (part.empty()) {
                    return std::nullopt;
                }

                for (const unsigned char ch : part) {
                    if (!std::isdigit(ch)) {
                        return std::nullopt;
                    }
                }

                try {
                    octets[i] = std::stoi(part);
                } catch (...) {
                    return std::nullopt;
                }

                if (octets[i] < 0 || octets[i] > 255) {
                    return std::nullopt;
                }
            }

            if (stream.rdbuf()->in_avail() != 0) {
                return std::nullopt;
            }

            return octets;
        }

        std::string ToLower(std::string value) {
            std::transform(value.begin(), value.end(), value.begin(), [](const unsigned char ch) {
                return static_cast<char>(std::tolower(ch));
            });
            return value;
        }

        std::string NarrowWideString(const wchar_t* value) {
            if (value == nullptr || *value == L'\0') {
                return {};
            }

            const auto requiredSize = WideCharToMultiByte(CP_UTF8, 0, value, -1, nullptr, 0, nullptr, nullptr);
            if (requiredSize <= 1) {
                return {};
            }

            std::string result(requiredSize, '\0');
            const auto written = WideCharToMultiByte(
                CP_UTF8, 0, value, -1, result.data(), requiredSize, nullptr, nullptr);
            if (written <= 1) {
                return {};
            }

            result.resize(written - 1);
            return result;
        }

        bool IsLinkLocalIPv4(const std::array<int, 4>& octets) {
            return octets[0] == 169 && octets[1] == 254;
        }

        bool IsLanInterfaceType(const unsigned long interfaceType) {
            return interfaceType == IF_TYPE_ETHERNET_CSMACD || interfaceType == IF_TYPE_IEEE80211;
        }

        bool IsDenylistedAdapter(const LocalAddressCandidate& candidate) {
            const auto haystack = ToLower(candidate.friendlyName + " " + candidate.description);
            static constexpr std::array kPatterns = {
                "wsl",
                "vethernet",
                "virtualbox",
                "vmware",
                "hyper-v",
                "docker",
                "loopback",
                "openvpn",
                "wireguard",
                "tailscale",
                "zerotier",
                "npcap",
            };

            return std::ranges::any_of(kPatterns, [&haystack](const char* pattern) {
                return haystack.find(pattern) != std::string::npos;
            });
        }

        bool IsUsablePrivateCandidate(const LocalAddressCandidate& candidate) {
            if (candidate.operStatus != IfOperStatusUp) {
                return false;
            }

            if (candidate.interfaceType == IF_TYPE_SOFTWARE_LOOPBACK || candidate.interfaceType == IF_TYPE_TUNNEL) {
                return false;
            }

            return IsPrivateIPv4(candidate.address);
        }

        std::vector<LocalAddressCandidate> EnumerateLocalAddressCandidates() {
            ULONG bufferSize = 0;
            constexpr ULONG flags = GAA_FLAG_SKIP_ANYCAST | GAA_FLAG_SKIP_MULTICAST | GAA_FLAG_SKIP_DNS_SERVER;

            auto result = GetAdaptersAddresses(AF_INET, flags, nullptr, nullptr, &bufferSize);
            if (result != ERROR_BUFFER_OVERFLOW || bufferSize == 0) {
                return {};
            }

            std::vector<std::byte> buffer(bufferSize);
            auto* adapters = reinterpret_cast<IP_ADAPTER_ADDRESSES*>(buffer.data());
            result = GetAdaptersAddresses(AF_INET, flags, nullptr, adapters, &bufferSize);
            if (result != NO_ERROR) {
                return {};
            }

            std::vector<LocalAddressCandidate> candidates;
            for (auto* adapter = adapters; adapter != nullptr; adapter = adapter->Next) {
                const auto friendlyName = NarrowWideString(adapter->FriendlyName);
                const auto description = NarrowWideString(adapter->Description);

                for (auto* unicast = adapter->FirstUnicastAddress; unicast != nullptr; unicast = unicast->Next) {
                    if (unicast->Address.lpSockaddr == nullptr || unicast->Address.lpSockaddr->sa_family != AF_INET) {
                        continue;
                    }

                    const auto* address = reinterpret_cast<const sockaddr_in*>(unicast->Address.lpSockaddr);
                    char addressBuffer[INET_ADDRSTRLEN] = {};
                    if (InetNtopA(AF_INET, const_cast<IN_ADDR*>(&address->sin_addr), addressBuffer,
                                  static_cast<DWORD>(std::size(addressBuffer))) == nullptr) {
                        continue;
                    }

                    candidates.push_back(LocalAddressCandidate{
                        .address = addressBuffer,
                        .friendlyName = friendlyName,
                        .description = description,
                        .interfaceIndex = static_cast<unsigned long>(adapter->IfIndex),
                        .metric = static_cast<unsigned long>(adapter->Ipv4Metric),
                        .interfaceType = static_cast<unsigned long>(adapter->IfType),
                        .operStatus = static_cast<unsigned long>(adapter->OperStatus),
                    });
                }
            }

            return candidates;
        }
    }

    bool IsPrivateIPv4(const std::string& address) {
        const auto octets = ParseIPv4Octets(address);
        if (!octets.has_value()) {
            return false;
        }

        if (IsLinkLocalIPv4(*octets)) {
            return false;
        }

        return (*octets)[0] == 10 ||
               ((*octets)[0] == 172 && (*octets)[1] >= 16 && (*octets)[1] <= 31) ||
               ((*octets)[0] == 192 && (*octets)[1] == 168);
    }

    std::optional<std::string> SelectPreferredPrivateIPv4(const std::vector<LocalAddressCandidate>& candidates,
                                                          const std::optional<unsigned long> preferredInterfaceIndex) {
        std::vector<const LocalAddressCandidate*> filtered;
        filtered.reserve(candidates.size());
        for (const auto& candidate : candidates) {
            if (!IsUsablePrivateCandidate(candidate) || IsDenylistedAdapter(candidate)) {
                continue;
            }
            filtered.push_back(&candidate);
        }

        if (filtered.empty()) {
            return std::nullopt;
        }

        std::ranges::sort(filtered, [preferredInterfaceIndex](const LocalAddressCandidate* left,
                                                              const LocalAddressCandidate* right) {
            const auto leftIsLan = IsLanInterfaceType(left->interfaceType);
            const auto rightIsLan = IsLanInterfaceType(right->interfaceType);
            if (leftIsLan != rightIsLan) {
                return leftIsLan > rightIsLan;
            }

            const auto leftPreferred = preferredInterfaceIndex.has_value() && left->interfaceIndex == preferredInterfaceIndex.value();
            const auto rightPreferred = preferredInterfaceIndex.has_value() && right->interfaceIndex == preferredInterfaceIndex.value();
            if (leftPreferred != rightPreferred) {
                return leftPreferred > rightPreferred;
            }

            if (left->metric != right->metric) {
                return left->metric < right->metric;
            }

            if (left->interfaceIndex != right->interfaceIndex) {
                return left->interfaceIndex < right->interfaceIndex;
            }

            return left->address < right->address;
        });

        return filtered.front()->address;
    }

    std::optional<std::string> GetLocalPrivateIPv4() {
        try {
            std::optional<unsigned long> preferredInterfaceIndex;
            unsigned long bestInterface = 0;
            IN_ADDR bestTarget{};
            if (InetPtonA(AF_INET, "8.8.8.8", &bestTarget) == 1 &&
                GetBestInterface(bestTarget.S_un.S_addr, &bestInterface) == NO_ERROR) {
                preferredInterfaceIndex = bestInterface;
            }

            return SelectPreferredPrivateIPv4(EnumerateLocalAddressCandidates(), preferredInterfaceIndex);
        } catch (...) {
            return std::nullopt;
        }
    }
}
