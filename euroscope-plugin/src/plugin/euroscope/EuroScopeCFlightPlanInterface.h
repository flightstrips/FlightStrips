#pragma once
#include <string>

namespace FlightStrips::euroscope {

/// Pure-virtual interface wrapping EuroScopePlugIn::CFlightPlan.
/// Production code uses EuroScopeCFlightPlanWrapper; tests use a GoogleMock.
class EuroScopeCFlightPlanInterface {
public:
    virtual ~EuroScopeCFlightPlanInterface() = default;

    virtual std::string GetCallsign() const = 0;
    virtual std::string GetOrigin() const = 0;
    virtual std::string GetDestination() const = 0;
    virtual std::string GetRoute() const = 0;
    virtual std::string GetAircraftType() const = 0;
    virtual std::string GetAirline() const = 0;
    virtual std::string GetAssignedSquawk() const = 0;
    virtual std::string GetScratchPadString() const = 0;
    virtual int GetClearedAltitude() const = 0;
    virtual bool IsTrackedByMe() const = 0;
    virtual void SetScratchPadString(const std::string& s) = 0;
    virtual void SetSquawk(const std::string& squawk) = 0;
};

} // namespace FlightStrips::euroscope
