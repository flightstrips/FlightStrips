#pragma once
#include "flightplan/FlightPlanService.h"
#include "handlers/MessageHandler.h"
#include "plugin/FlightStripsPlugin.h"
#include "websocket/WebSocketService.h"
#include "stands/StandService.h"

namespace FlightStrips::messages {

    class MessageService final : public handlers::MessageHandler {
    public:
        MessageService(const std::shared_ptr<FlightStripsPlugin> &m_plugin,
            const std::shared_ptr<websocket::WebSocketService> &m_web_socket_service,
            const std::shared_ptr<flightplan::FlightPlanService> &m_flight_plan_service,
            const std::shared_ptr<stands::StandService> &m_stand_service)
            : m_plugin(m_plugin),
              m_webSocketService(m_web_socket_service),
              m_flightPlanService(m_flight_plan_service),
              m_standService(m_stand_service) {
        }

        void OnMessages(const std::vector<nlohmann::json> &messages) override;
    private:
        std::shared_ptr<FlightStripsPlugin> m_plugin;
        std::shared_ptr<websocket::WebSocketService> m_webSocketService;
        std::shared_ptr<flightplan::FlightPlanService> m_flightPlanService;
        std::shared_ptr<stands::StandService> m_standService;

        void HandleMessage(const nlohmann::json &message) const;
        void HandleSessionInfoEvent(const SessionInfoEvent& event) const;
    };

}
