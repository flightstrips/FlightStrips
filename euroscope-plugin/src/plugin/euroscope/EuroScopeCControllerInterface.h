#pragma once
#include <string>

namespace FlightStrips::euroscope {

/// Pure-virtual interface wrapping EuroScopePlugIn::CController.
class EuroScopeCControllerInterface {
public:
    virtual ~EuroScopeCControllerInterface() = default;

    virtual std::string GetCallsign() const = 0;
    virtual std::string GetPositionId() const = 0;
    virtual int GetFrequency() const = 0;
    virtual bool IsValid() const = 0;
};

} // namespace FlightStrips::euroscope
