#pragma once
#include "Colors.h"
#include "Graphics.h"
#include "vacs/VacsClient.h"
#include "euroscope/EuroScopePlugIn.h"

namespace FlightStrips::graphics {
    struct VacsButton { RECT rect; int objectId; std::string label; };
    struct VacsLayout {
        RECT dot, status, enabled, checkbox, preference;
        std::vector<VacsButton> buttons;
    };
    [[nodiscard]] VacsLayout CalculateVacsLayout(int left, int top, bool mini, const vacs::View& view);
    [[nodiscard]] int CalculateVacsMiniHeight(const vacs::View& view);
    void DrawVacsPanel(EuroScopePlugIn::CRadarScreen* screen, Graphics& graphics, const Colors& colors,
                       int left, int top, bool mini, bool enabled, bool showMini, const vacs::View& view);
}
