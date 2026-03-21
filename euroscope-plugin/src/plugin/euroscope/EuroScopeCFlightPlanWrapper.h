#pragma once
#include <string>
#include "EuroScopeCFlightPlanInterface.h"
#include <euroscope/EuroScopePlugIn.h>

namespace FlightStrips::euroscope {

/// Production implementation of EuroScopeCFlightPlanInterface delegating
/// to the real EuroScopePlugIn::CFlightPlan object.
class EuroScopeCFlightPlanWrapper final : public EuroScopeCFlightPlanInterface {
public:
    explicit EuroScopeCFlightPlanWrapper(EuroScopePlugIn::CFlightPlan flightPlan)
        : m_flightPlan(flightPlan) {}

    std::string GetCallsign() const override {
        return m_flightPlan.GetCallsign();
    }

    std::string GetOrigin() const override {
        return m_flightPlan.GetFlightPlanData().GetOrigin();
    }

    std::string GetDestination() const override {
        return m_flightPlan.GetFlightPlanData().GetDestination();
    }

    std::string GetRoute() const override {
        return m_flightPlan.GetFlightPlanData().GetRoute();
    }

    std::string GetAircraftType() const override {
        return m_flightPlan.GetFlightPlanData().GetAircraftFPType();
    }

    std::string GetAirline() const override {
        // Airline is the first 3 chars of the callsign
        const std::string cs = m_flightPlan.GetCallsign();
        return cs.size() >= 3 ? cs.substr(0, 3) : cs;
    }

    std::string GetAssignedSquawk() const override {
        return m_flightPlan.GetControllerAssignedData().GetSquawk();
    }

    std::string GetScratchPadString() const override {
        return m_flightPlan.GetControllerAssignedData().GetFlightStripAnnotation(0);
    }

    int GetClearedAltitude() const override {
        return m_flightPlan.GetControllerAssignedData().GetClearedAltitude();
    }

    bool IsTrackedByMe() const override {
        return m_flightPlan.GetTrackingControllerIsMe();
    }

    void SetScratchPadString(const std::string& s) override {
        m_flightPlan.GetControllerAssignedData().SetFlightStripAnnotation(0, s.c_str());
    }

    void SetSquawk(const std::string& squawk) override {
        m_flightPlan.GetControllerAssignedData().SetSquawk(squawk.c_str());
    }

private:
    EuroScopePlugIn::CFlightPlan m_flightPlan;
};

} // namespace FlightStrips::euroscope
