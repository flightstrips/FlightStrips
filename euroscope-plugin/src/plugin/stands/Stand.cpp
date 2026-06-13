#include "Stand.h"

#include <utility>

namespace FlightStrips {
    stands::Stand::Stand(std::string name, std::string airport, EuroScopePlugIn::CPosition position,
                                  double radius) : name(std::move(name)), airport(std::move(airport)), position(position), radius(radius) { }

    std::optional<stands::Stand> stands::Stand::FromLine(const std::string& rawLine) {
        // Only genuine stand records start with "STAND:". This deliberately excludes
        // the "STANDLIST:" header line that GroundRadar stand files begin with, which
        // would otherwise be parsed with the fixed offsets below and throw.
        constexpr std::string_view prefix = "STAND:";
        if (!rawLine.starts_with(prefix)) {
            return std::nullopt;
        }

        std::string line = rawLine.substr(prefix.size());

        const std::size_t airportEnd = line.find_first_of(':');
        if (airportEnd == std::string::npos) {
            return std::nullopt;
        }
        std::string airport = line.substr(0, airportEnd);
        line = line.substr(airportEnd + 1);

        const std::size_t nameEnd = line.find_first_of(':');
        if (nameEnd == std::string::npos) {
            return std::nullopt;
        }
        std::string name = line.substr(0, nameEnd);
        line = line.substr(nameEnd + 1);

        // Remaining layout is <lat[14]><sep><lon[14]><sep><radius>, so the shortest
        // valid record has the radius starting at index 30. Anything shorter is
        // malformed and must be skipped rather than fed to substr/stod.
        if (line.size() < 31) {
            return std::nullopt;
        }

        const std::string lat = line.substr(0, 14);
        const std::string lon = line.substr(15, 14);

        double radius;
        try {
            radius = std::stod(line.substr(30));
        } catch (...) {
            return std::nullopt;
        }

        EuroScopePlugIn::CPosition pos;
        pos.LoadFromStrings(lon.c_str(), lat.c_str());
        return Stand{name, airport, pos, radius};
    }

    std::string stands::Stand::GetAirport() const {
        return this->airport;
    }

    EuroScopePlugIn::CPosition stands::Stand::GetPosition() const {
        return this->position;
    }

    std::string stands::Stand::GetName() const {
        return this->name;
    }

    double stands::Stand::GetRadius() const {
        return this->radius;
    }
}

