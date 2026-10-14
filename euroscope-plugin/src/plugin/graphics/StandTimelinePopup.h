#pragma once

#include <string>
#include <vector>

#include "Colors.h"
#include "Graphics.h"
#include "euroscope/EuroScopePlugIn.h"

namespace FlightStrips::graphics {
    struct StandTimelineRow {
        std::string stand;
        std::string callsign;
        std::string detail;
        int startOffsetMinutes{0};
        int endOffsetMinutes{0};
        bool active{false};
        bool blocked{false};
        std::string sortKey;
    };

    struct StandTimelineData {
        std::vector<StandTimelineRow> rows;
        std::string updatedAt;
        std::string error;
    };

    struct StandTimelinePopupState {
        bool isOpen{false};
        int posX{400};
        int posY{400};
    };

    struct StandTimelinePopupSize {
        int width;
        int height;
    };

    [[nodiscard]] StandTimelineData FetchStandTimeline(
        const std::string& baseUrl,
        const std::string& airport,
        const std::string& sessionName,
        const std::string& accessToken);

    [[nodiscard]] StandTimelinePopupSize CalculateStandTimelinePopupSize(const StandTimelineData& data);

    void DrawStandTimelinePopup(
        EuroScopePlugIn::CRadarScreen& screen,
        Graphics& graphics,
        const Colors& colors,
        const StandTimelinePopupState& state,
        const StandTimelineData& data,
        bool loading);
}
