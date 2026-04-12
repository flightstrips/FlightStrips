#pragma once
#include <string>

namespace FlightStrips::graphics {
    struct PdcClearancePopupState {
        bool isOpen{false};
        std::string callsign;
        int posX{400};
        int posY{400};
    };
}
