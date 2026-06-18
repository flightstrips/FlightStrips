#include "AirlineCallsignService.h"

namespace FlightStrips::flightplan {
    namespace {
        const std::regex kCallsignEqualsPattern(R"(CALLSIGN\s+([^=|/]+?)=)", std::regex::icase);
        const std::regex kCsPattern(R"(\bCS\s*=\s*([^|=/]+))", std::regex::icase);
    }

    AirlineCallsignService::AirlineCallsignService(std::string filePath) {
        LoadFromFile(filePath);
    }

    std::string AirlineCallsignService::ResolveSpokenCallsign(const std::string& callsign, const std::string& remarks) const {
        if (const auto fromRemarks = ResolveFromRemarks(remarks); !fromRemarks.empty()) {
            return fromRemarks;
        }

        return ResolveFromCallsignMap(callsign, telephonyByIcao_);
    }

    size_t AirlineCallsignService::Size() const {
        return telephonyByIcao_.size();
    }

    void AirlineCallsignService::LoadFromFile(const std::string& filePath) {
        if (filePath.empty()) {
            return;
        }

        std::ifstream input(filePath);
        if (!input.is_open()) {
            return;
        }

        std::string line;
        while (std::getline(input, line)) {
            if (!line.empty() && line.back() == '\r') {
                line.pop_back();
            }

            const auto trimmed = Trim(line);
            if (trimmed.empty() || trimmed[0] == ';') {
                continue;
            }

            std::vector<std::string> columns;
            size_t start = 0;
            while (start <= trimmed.size()) {
                const auto tab = trimmed.find('\t', start);
                columns.push_back(trimmed.substr(start, tab == std::string::npos ? std::string::npos : tab - start));
                if (tab == std::string::npos) {
                    break;
                }
                start = tab + 1;
            }

            if (columns.size() < 3) {
                continue;
            }

            auto icao = ToUpper(Trim(columns[0]));
            auto telephony = NormalizeWhitespace(Trim(columns[2]));
            if (icao.size() != 3 || telephony.empty()) {
                continue;
            }

            telephonyByIcao_.insert_or_assign(std::move(icao), std::move(telephony));
        }
    }

    std::string AirlineCallsignService::ResolveFromRemarks(const std::string& remarks) {
        if (remarks.empty()) {
            return "";
        }

        if (const auto fromBackslashPattern = ResolveCallsignBackslashPattern(remarks); !fromBackslashPattern.empty()) {
            return fromBackslashPattern;
        }

        std::smatch match;
        if (std::regex_search(remarks, match, kCallsignEqualsPattern) && match.size() > 1) {
            return NormalizeWhitespace(Trim(match[1].str()));
        }
        if (std::regex_search(remarks, match, kCsPattern) && match.size() > 1) {
            return NormalizeWhitespace(Trim(match[1].str()));
        }

        return "";
    }

    std::string AirlineCallsignService::ResolveCallsignBackslashPattern(const std::string& remarks) {
        const auto uppercaseRemarks = ToUpper(remarks);
        const auto callsignIndex = uppercaseRemarks.find("CALLSIGN");
        if (callsignIndex == std::string::npos) {
            return "";
        }

        const auto backslashIndex = remarks.find('\\', callsignIndex);
        if (backslashIndex == std::string::npos) {
            return "";
        }

        auto valueStart = backslashIndex + 1;
        while (valueStart < remarks.size() &&
               (std::isspace(static_cast<unsigned char>(remarks[valueStart])) || remarks[valueStart] == '\\')) {
            ++valueStart;
        }

        if (valueStart >= remarks.size()) {
            return "";
        }

        auto valueEnd = remarks.find("/V/", valueStart);
        if (valueEnd == std::string::npos) {
            valueEnd = remarks.find('/', valueStart);
        }
        if (valueEnd == std::string::npos) {
            valueEnd = remarks.size();
        }

        return NormalizeWhitespace(Trim(remarks.substr(valueStart, valueEnd - valueStart)));
    }

    std::string AirlineCallsignService::ResolveFromCallsignMap(
        const std::string& callsign,
        const std::unordered_map<std::string, std::string>& telephonyByIcao) {
        if (callsign.size() < 4 || !std::isdigit(static_cast<unsigned char>(callsign[3]))) {
            return "";
        }

        const auto prefix = ToUpper(callsign.substr(0, 3));
        const auto it = telephonyByIcao.find(prefix);
        if (it == telephonyByIcao.end()) {
            return "";
        }

        return it->second;
    }

    std::string AirlineCallsignService::NormalizeWhitespace(std::string value) {
        std::string normalized;
        normalized.reserve(value.size());

        bool previousWasSpace = false;
        for (const unsigned char ch : value) {
            if (std::isspace(ch)) {
                if (!previousWasSpace) {
                    normalized.push_back(' ');
                    previousWasSpace = true;
                }
                continue;
            }

            normalized.push_back(static_cast<char>(ch));
            previousWasSpace = false;
        }

        return Trim(normalized);
    }

    std::string AirlineCallsignService::Trim(const std::string& value) {
        const auto start = value.find_first_not_of(" \t\n\r");
        if (start == std::string::npos) {
            return "";
        }

        const auto end = value.find_last_not_of(" \t\n\r");
        return value.substr(start, end - start + 1);
    }

    std::string AirlineCallsignService::ToUpper(std::string value) {
        std::ranges::transform(value, value.begin(), [](const unsigned char c) {
            return static_cast<char>(std::toupper(c));
        });
        return value;
    }
}
