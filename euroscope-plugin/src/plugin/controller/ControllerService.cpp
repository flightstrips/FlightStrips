#include "ControllerService.h"

namespace FlightStrips::controller {
    void ControllerService::ControllerPositionUpdateEvent(EuroScopePlugIn::CController controller) {
        if (!m_webSocketService->ShouldSend()) return;
        const auto primaryFrequency = std::format("{:.3f}", controller.GetPrimaryFrequency());
        m_webSocketService->SendEvent(ControllerOnlineEvent(std::string(controller.GetCallsign()), primaryFrequency));
    }

    void ControllerService::ControllerDisconnectEvent(EuroScopePlugIn::CController controller) {
        if (!m_webSocketService->ShouldSend()) return;
        m_webSocketService->SendEvent(ControllerOfflineEvent(std::string(controller.GetCallsign())));
    }
}
