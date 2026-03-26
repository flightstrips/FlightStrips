#pragma once

namespace FlightStrips::flightplan {
    struct CdmState {
        std::string eobt{};
        std::string tobt{};
        std::string req_tobt{};
        std::string req_tobt_source{};
        std::string tobt_confirmed_by{};
        std::string tsat{};
        std::string ttot{};
        std::string ctot{};
        std::string asrt{};
        std::string tsac{};
        std::string asat{};
        std::string status{};
        std::string manual_ctot{};
        std::string deice_type{};
        std::string ecfmp_id{};
        std::string phase{};
    };

    struct FlightPlan {
    public:
        std::string squawk{};
        std::string stand{};
        std::string tracking_controller{};
        CdmState cdm{};
    };
}
