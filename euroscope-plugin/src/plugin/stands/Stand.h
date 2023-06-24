
#pragma once

namespace FlightStrips::stands {
    class Stand {
    public:
        static Stand FromLine(std::string line);

    public:
        Stand(std::string name, std::string airport, EuroScopePlugIn::CPosition position, double radius);

        std::string GetName();
        std::string GetAirport();
        EuroScopePlugIn::CPosition GetPosition();
        double GetRadius();

    private:
        std::string name;
        std::string airport;
        EuroScopePlugIn::CPosition position;
        double radius;
    };
}