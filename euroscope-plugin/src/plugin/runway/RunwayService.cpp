#include "RunwayService.h"
#include "websocket/WebSocketService.h"

namespace FlightStrips::runway {
    void RunwayService::Online() {
        SendRunwayEvent();
    }

    void RunwayService::OnAirportRunwayActivityChanged() {
        SendRunwayEvent();
    }


    std::vector<Runway> RunwayService::GetActiveRunways(const char *airport) const {
        std::vector<Runway> active;

        auto it = m_plugin->SectorFileElementSelectFirst(EuroScopePlugIn::SECTOR_ELEMENT_RUNWAY);
        while (it.IsValid()) {
            if (strncmp(it.GetAirportName(), airport, 4) == 0) {
                for (int i = 0; i < 2; i++) {
                    const auto isDeparture = it.IsElementActive(true, i);
                    const auto isArrival = it.IsElementActive(false, i);
                    if (isDeparture || isArrival) {
                        active.push_back(Runway(std::string(it.GetRunwayName(i)), isDeparture, isArrival));
                    }
                }
            }

            it = m_plugin->SectorFileElementSelectNext(it, EuroScopePlugIn::SECTOR_ELEMENT_RUNWAY);
        }

        return active;
    }

    void RunwayService::SendRunwayEvent() const {
        if (!m_websocketService->IsConnected()) return;
        const auto airport = m_plugin->GetConnectionState().relevant_airport;
        if (airport.empty()) return;

        const auto event = RunwayEvent(GetActiveRunways(airport.c_str()));
        m_websocketService->SendEvent(event);
    }
}
