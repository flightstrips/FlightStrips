
#pragma once

namespace FlightStrips::stands {
    class Stand {
    public:
        static Stand FromLine(std::string line);

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