#pragma once
#include "handlers/ConnectionEventHandler.h"
#include "plugin/FlightStripsPlugin.h"
#include "websocket/Events.h"

namespace FlightStrips::websocket {
    class WebSocketService;
}

namespace FlightStrips::runway {

    class RunwayService final : public handlers::ConnectionEventHandler, handlers::AirportRunwaysChangedEvent {
    public:
        RunwayService(const std::shared_ptr<websocket::WebSocketService> &m_websocket_service,
            const std::shared_ptr<FlightStripsPlugin> &m_plugin)
            : m_websocketService(m_websocket_service),
              m_plugin(m_plugin) {
        }

        void Online() override;
        void OnAirportRunwayActivityChanged() override;

    private:
        std::shared_ptr<websocket::WebSocketService> m_websocketService;
        std::shared_ptr<FlightStripsPlugin> m_plugin;


        std::vector<Runway> GetActiveRunways(const char* airport) const;
        void SendRunwayEvent() const;
    };

}
