#include "Stand.h"

#include <utility>

namespace FlightStrips {
    stands::Stand::Stand(std::string name, std::string airport, EuroScopePlugIn::CPosition position,
                                  double radius) : name(std::move(name)), airport(std::move(airport)), position(position), radius(radius) { }

    stands::Stand stands::Stand::FromLine(std::string line) {
        line = line.substr(6);
        std::size_t airportEnd = line.find_first_of(':');
        std::string airport = line.substr(0, airportEnd);
        line = line.substr(airportEnd + 1);

        std::size_t nameEnd = line.find_first_of(':');
        std::string name = line.substr(0, nameEnd);
        line = line.substr(nameEnd + 1);

        std::string lat = line.substr(0, 14);
        std::string lon = line.substr(15, 14);
        double radius = std::stod(line.substr(30));

        EuroScopePlugIn::CPosition pos;
        pos.LoadFromStrings(lon.c_str(), lat.c_str());
        return {name, airport, pos, radius};
    }

    std::string stands::Stand::GetAirport() {
        return this->airport;
    }

    EuroScopePlugIn::CPosition stands::Stand::GetPosition() {
        return this->position;
    }

    std::string stands::Stand::GetName() {
        return this->name;
    }

    double stands::Stand::GetRadius() {
        return this->radius;
    }
}

