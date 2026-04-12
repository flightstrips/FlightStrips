#pragma once
#include <string>

namespace FlightStrips::graphics {
    struct PdcClearancePopupState {
        bool isOpen{false};
        std::string callsign;
        std::string clearanceRemarks;
        int posX{400};
        int posY{400};
    };
}
