#include "ControllerService.h"

namespace FlightStrips::controller {
    void ControllerService::ControllerPositionUpdateEvent(EuroScopePlugIn::CController controller) {
        const auto primaryFrequency = std::format("{:.3f}", controller.GetPrimaryFrequency());
        const auto c = Controller{primaryFrequency};
        const auto [value, inserted] = m_controllers.insert({controller.GetCallsign(), c});

        bool shouldSend = true;

        if (!inserted) {
            if (value->second.position != primaryFrequency) {
                value->second.position = primaryFrequency;
            } else {
                shouldSend = false;
            }
        }

        if (!m_webSocketService->ShouldSend()) return;
        if (!shouldSend) return;

        m_webSocketService->SendEvent(ControllerOnlineEvent(std::string(controller.GetCallsign()), primaryFrequency));
    }

    void ControllerService::ControllerDisconnectEvent(EuroScopePlugIn::CController controller) {
        if (!m_webSocketService->ShouldSend()) return;
        m_webSocketService->SendEvent(ControllerOfflineEvent(std::string(controller.GetCallsign())));
    }
}
