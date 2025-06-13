#pragma once
#include "Controller.h"
#include "handlers/ControllerEventHandler.h"
#include "websocket/WebSocketService.h"

namespace FlightStrips::controller {
    class ControllerService final : public handlers::ControllerEventHandler {
    public:
        explicit ControllerService(const std::shared_ptr<websocket::WebSocketService> &m_web_socket_service)
            : m_webSocketService(m_web_socket_service) {
        }

        void ControllerPositionUpdateEvent(EuroScopePlugIn::CController controller) override;
        void ControllerDisconnectEvent(EuroScopePlugIn::CController controller) override;
    private:
        std::shared_ptr<websocket::WebSocketService> m_webSocketService;
        std::unordered_map<std::string, Controller> m_controllers = {};
    };
}
