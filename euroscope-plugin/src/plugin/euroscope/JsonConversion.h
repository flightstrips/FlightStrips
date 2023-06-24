//
// Created by fsr19 on 26/05/2023.
//

#pragma once

#include <nlohmann/json.hpp>
#include "euroscope/EuroScopePlugIn.h"
#include <locale>
#include <codecvt>

using json = nlohmann::json;

namespace EuroScopePlugIn {
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

    void to_json(json& j, const CFlightPlanData& data) {
        j = json
            {
                //{ "callsign", data() },
                //{ "isReceived", data.IsReceived() },
                //{ "isAmended", data.IsAmended() },
                { "planType", data.GetPlanType() },
                //{ "aircraftInfo", data.GetAircraftInfo() },
                { "aircraftWtc", toCharString(data.GetAircraftWtc()) },
                { "aircraftType", toCharString(data.GetAircraftType()) },
                //{ "engineNumber", data.GetEngineNumber() },
                //{ "engineType", std::string(1, data.GetEngineType()) },
                { "capibilities", toCharString(data.GetCapibilities()) },
                //{ "isRvsm",                 data.IsRvsm() },
                //{ "manufacturerType",       data.GetManufacturerType() },
                { "aircraftFPType",         data.GetAircraftFPType() },
                //{ "trueAirspeed",           data.GetTrueAirspeed() },
                { "origin",                 data.GetOrigin() },
                { "finalAltitude",          data.GetFinalAltitude() },
                { "destination",            data.GetDestination() },
                { "alternate",              data.GetAlternate() },
                { "remarks", toUtf8(data.GetRemarks()) },
                { "communicationType", toCharString(data.GetCommunicationType()) },
                { "route", toUtf8(data.GetRoute()) },
                { "sidName",                data.GetSidName() },
                { "starName",               data.GetStarName() },
                { "departureRwy",           data.GetDepartureRwy() },
                { "arrivalRwy",             data.GetArrivalRwy() },
                { "estimatedDepartureTime", data.GetEstimatedDepartureTime() },
                //{ "ActualDepartureTime",    data.GetActualDepartureTime() },
                //{ "enrouteHours",           data.GetEnrouteHours() },
                //{ "enrouteMinutes",         data.GetEnrouteMinutes() },
                //{ "fuelHours",              data.GetFuelHours() },
                //{ "fuelMinutes",            data.GetFuelMinutes() },
            };
    }

}