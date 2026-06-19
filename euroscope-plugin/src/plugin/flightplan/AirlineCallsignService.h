#pragma once

#include <string>
#include <unordered_map>

namespace FlightStrips::flightplan {
    class AirlineCallsignService {
    public:
        explicit AirlineCallsignService(std::string filePath);

        [[nodiscard]] std::string ResolveSpokenCallsign(const std::string& callsign, const std::string& remarks) const;
        [[nodiscard]] size_t Size() const;

    private:
        std::unordered_map<std::string, std::string> telephonyByIcao_;

        void LoadFromFile(const std::string& filePath);

        static std::string ResolveFromRemarks(const std::string& remarks);
        static std::string ResolveCallsignBackslashPattern(const std::string& remarks);
        static std::string ResolveFromCallsignMap(const std::string& callsign,
                                                  const std::unordered_map<std::string, std::string>& telephonyByIcao);
        static std::string NormalizeWhitespace(std::string value);
        static std::string Trim(const std::string& value);
        static std::string ToUpper(std::string value);
    };
}
