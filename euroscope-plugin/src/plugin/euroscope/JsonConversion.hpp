#pragma once

#include <nlohmann/json.hpp>
#include "euroscope/EuroScopePlugIn.h"
#include <locale>
#include <codecvt>

using json = nlohmann::json;

namespace FlightStrips::euroscope {
    static std::string toUtf8(const char* iso88591String) {
        const int codePage = 1251;
        int size = MultiByteToWideChar(codePage, 0, iso88591String, -1, nullptr, 0);
        std::wstring wstring(size, 0);
        MultiByteToWideChar(codePage, 0, iso88591String, -1, &wstring[0], size);

        size = WideCharToMultiByte(CP_UTF8, 0, &wstring[0], -1, nullptr, 0, nullptr, nullptr) - 1;

        if (size == -1) {
            return {};
        }

        std::string str(size, 0);
        WideCharToMultiByte(CP_UTF8, 0, &wstring[0], -1, &str[0], size, nullptr, nullptr);

        return str;
    }

    static std::string toCharString(char character) {
        if (character == 0) {
            return "?";
        }

        return {(char)toupper(character)};
    }

    void to_json(json& j, const EuroScopePlugIn::CFlightPlanData& data) {
        j = json
            {
                { "planType", data.GetPlanType() },
                { "aircraftWtc", toCharString(data.GetAircraftWtc()) },
                { "aircraftType", toCharString(data.GetAircraftType()) },
                { "capibilities", toCharString(data.GetCapibilities()) },
                { "aircraftFPType",         data.GetAircraftFPType() },
                { "origin",                 data.GetOrigin() },
                { "finalAltitude",          data.GetFinalAltitude() },
                { "destination",            data.GetDestination() },
                { "alternate",              data.GetAlternate() },
                { "remarks", toUtf8(data.GetRemarks()) },
                { "communicationType", toCharString(data.GetCommunicationType()) },
                { "route", toUtf8(data.GetRoute()) },
                { "sidName",                data.GetSidName() },
                { "starName",               toUtf8(data.GetStarName()) },
                { "departureRwy",           data.GetDepartureRwy() },
                { "arrivalRwy",             data.GetArrivalRwy() },
                { "estimatedDepartureTime", data.GetEstimatedDepartureTime() }
            };
    }
}