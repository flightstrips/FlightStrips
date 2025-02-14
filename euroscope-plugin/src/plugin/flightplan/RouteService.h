#pragma once
#include <optional>

#include "plugin/FlightStripsPluginInterface.h"

namespace FlightStrips::flightplan {
    class RouteService {
    public:
        explicit RouteService(const std::shared_ptr<FlightStripsPluginInterface> &m_plugin)
            : m_plugin(m_plugin) {
        }


        /// <summary>
        /// Modifies the route to set the specified SID.
        /// </summary>
        void SetSid(std::string& route, const std::string& sid, const std::string& airport) const;


        /// <summary>
        /// Modifies the route to set the specified departure runway.
        /// </summary>
        void SetDepartureRunway(std::string& route, const std::string& departureRunway, const std::string& airport) const;
    private:
        struct Token {
            std::string token;
            size_t length;
        };

        std::shared_ptr<FlightStripsPluginInterface> m_plugin;

        static std::vector<std::string> GetBaseSids(const std::vector<Sid> &allSids);
        static std::string GetBaseSid(const std::string& sid);
        inline static bool EraseStartAirport(std::string& route, const std::string& airport);
        /// <summary>Erase token given that token is the start of the route.</summary>
        inline static void EraseToken(std::string& route, const Token& token);
        inline static std::optional<Token> GetNextToken(std::string& route);

        static void ToUpperCase(std::string &s) {
            for (char &c : s) { c = static_cast<char>(std::toupper(c)); }
        }


    };

    // TODO move to util class
    static std::string &ltrim(std::string &s) {
        s.erase(s.begin(), std::ranges::find_if(s, [](const int c) {return !std::isspace(c);}));
        return s;
    }
}
