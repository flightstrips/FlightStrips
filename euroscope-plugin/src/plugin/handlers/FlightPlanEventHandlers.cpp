#include "FlightPlanEventHandlers.h"

namespace FlightStrips::handlers {
    void FlightPlanEventHandlers::RegisterHandler(const std::shared_ptr<FlightPlanEventHandler> &handler) {
        this->m_handlers.push_back(handler);
    }

    void FlightPlanEventHandlers::Clear() {
        m_handlers.clear();
    }

    void FlightPlanEventHandlers::FlightPlanEvent(EuroScopePlugIn::CFlightPlan flightPlan) const {
        for (auto it = this->m_handlers.cbegin(); it != this->m_handlers.cend(); ++it) {
            (*it)->FlightPlanEvent(flightPlan);
        }
    }

    void FlightPlanEventHandlers::ControllerFlightPlanDataEvent(
            EuroScopePlugIn::CFlightPlan flightPlan,
            int dataType) const {
        for (auto it = this->m_handlers.cbegin(); it != this->m_handlers.cend(); ++it) {
            (*it)->ControllerFlightPlanDataEvent(flightPlan, dataType);
        }
    }

    void FlightPlanEventHandlers::FlightPlanDisconnectEvent(EuroScopePlugIn::CFlightPlan flightPlan) const {
        for (auto it = this->m_handlers.cbegin(); it != this->m_handlers.cend(); ++it) {
            (*it)->FlightPlanDisconnectEvent(flightPlan);
        }
    }
}
