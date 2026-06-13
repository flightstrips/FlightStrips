
#pragma once

#include <optional>
#include <string>

namespace FlightStrips::stands {
    class Stand {
    public:
        // Parses a single stands-file line. Returns std::nullopt for any line that
        // is not a well-formed "STAND:" entry (e.g. the "STANDLIST:" header line or
        // a truncated record), so a malformed file can never crash plugin start-up.
        static std::optional<Stand> FromLine(const std::string& line);

    public:
        Stand(std::string name, std::string airport, EuroScopePlugIn::CPosition position, double radius);

        std::string GetName() const;
        std::string GetAirport() const;
        EuroScopePlugIn::CPosition GetPosition() const;
        double GetRadius() const;

    private:
        std::string name;
        std::string airport;
        EuroScopePlugIn::CPosition position;
        double radius;
    };
}