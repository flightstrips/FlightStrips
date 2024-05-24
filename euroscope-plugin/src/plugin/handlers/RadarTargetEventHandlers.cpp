#include "RadarTargetEventHandlers.h"
namespace FlightStrips::handlers {
    void RadarTargetEventHandlers::RegisterHandler(const std::shared_ptr <RadarTargetEventHandler> &handler) {
        this->m_handlers.push_back(handler);
    }

    void RadarTargetEventHandlers::Clear() {
        m_handlers.clear();
    }

    void RadarTargetEventHandlers::RadarTargetPositionEvent(EuroScopePlugIn::CRadarTarget radarTarget) const {
        for (auto it = this->m_handlers.cbegin(); it != this->m_handlers.cend(); ++it) {
            (*it)->RadarTargetPositionEvent(radarTarget);
        }
    }
}

