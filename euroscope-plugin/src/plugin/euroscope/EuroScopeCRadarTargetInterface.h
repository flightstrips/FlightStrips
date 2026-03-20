#pragma once
#include <string>

namespace FlightStrips::euroscope {

/// Pure-virtual interface wrapping EuroScopePlugIn::CRadarTarget.
class EuroScopeCRadarTargetInterface {
public:
    virtual ~EuroScopeCRadarTargetInterface() = default;

    virtual std::string GetCallsign() const = 0;
    virtual double GetLatitude() const = 0;
    virtual double GetLongitude() const = 0;
    virtual int GetGroundSpeed() const = 0;
    virtual bool IsReported() const = 0;
};

} // namespace FlightStrips::euroscope
