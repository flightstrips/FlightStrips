#pragma once

#include <optional>
#include <string>
#include <string_view>

#include "Colors.h"
#include "Graphics.h"
#include "InfoScreenObjectIds.h"
#include "PdcClearancePopupState.h"
#include "euroscope/EuroScopePlugIn.h"

namespace FlightStrips {
    class FlightStripsPlugin;
    namespace flightplan {
        class FlightPlanService;
    }
    namespace runway {
        class RunwayService;
    }
}

namespace FlightStrips::graphics {
    struct PdcPopupData {
        std::string callsign;
        std::string gate;
        std::string destination;
        std::string aircraftType;
        std::string atis;
        std::string remarks;
        std::string pdcState;
        std::string runway;
        std::string sid;
        int heading{0};
        int clearedAltitude{0};
        std::string assignedSquawk;
        bool esCleared{false};
        bool runwayMismatch{false};
    };

    enum class PdcPopupPrimaryAction {
        None,
        IssueRequestedClearance,
        SetEuroscopeClearance,
    };

    [[nodiscard]] bool IsRequestedPdcState(std::string_view state);

    [[nodiscard]] auto ResolvePdcPopupPrimaryAction(std::string_view state, bool alreadyClear) -> PdcPopupPrimaryAction;

    [[nodiscard]] bool ShouldSendPdcRevertToVoice(std::string_view state);

    [[nodiscard]] std::optional<PdcPopupData> BuildPdcPopupData(
        const PdcClearancePopupState& state,
        FlightStripsPlugin& plugin,
        flightplan::FlightPlanService* flightPlanService,
        runway::RunwayService* runwayService);

    void DrawPdcPopup(
        EuroScopePlugIn::CRadarScreen& screen,
        Graphics& graphics,
        const Colors& colors,
        const PdcClearancePopupState& state,
        const PdcPopupData& data);
}
