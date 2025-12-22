#include "DeIceHandler.h"

#include "configuration/AppConfig.h"
#include "stands/StandService.h"

namespace FlightStrips::TagItems
{
    void DeIceHandler::Handle(EuroScopePlugIn::CFlightPlan FlightPlan, EuroScopePlugIn::CRadarTarget RadarTarget,
        int ItemCode, int TagData, char sItemString[16], int* pColorCode, COLORREF* pRGB, double* pFontSize)
    {
        const auto callsign = std::string(FlightPlan.GetCallsign());

        if (const auto entry = m_deiceLookup.find(callsign); entry != m_deiceLookup.end()) {
            const auto len = entry->second.copy(sItemString, 15);
            sItemString[len] = '\0';
            return;
        }

        const auto [order, ac_types, airlines, stands, fallback] = m_appConfig->GetDeIceConfig();
        for (const auto& action : order) {
            if (action == "ac_type") {
                const auto fpData = FlightPlan.GetFlightPlanData();
                const std::string acType = fpData.GetAircraftFPType();
                if (auto r = ac_types.find(acType); r != ac_types.end()) {
                    const auto len = r->second.copy(sItemString, 15);
                    sItemString[len] = '\0';
                    return;
                }
            } else if (action == "airline") {
                if (std::strlen(FlightPlan.GetCallsign()) < 3)
                {
                    continue;
                }

                std::string airline;
                airline.assign(FlightPlan.GetCallsign(), 0, 3);
                if (auto r = airlines.find(airline); r != airlines.end()) {
                    const auto len = r->second.copy(sItemString, 15);
                    sItemString[len] = '\0';
                    return;
                }
            } else if (action == "stand") {
                const auto stand = m_standService->GetStandFromFlightPlan(FlightPlan);
                if (stand == nullptr) {
                    continue;
                }

                const auto standName = stand->GetName();
                const auto prefix = standName.substr(0, 1);

                if (auto r = stands.find(prefix); r != stands.end())
                {
                    const auto len = r->second.copy(sItemString, 15);
                    sItemString[len] = '\0';
                    return;
                }
            }
        }

        const auto len = fallback.copy(sItemString, 15);
        sItemString[len] = '\0';
    }

    void DeIceHandler::FlightPlanDisconnectEvent(EuroScopePlugIn::CFlightPlan flightPlan)
    {
        const auto callsign = std::string(flightPlan.GetCallsign());
        m_deiceLookup.erase(callsign);
    }
}
