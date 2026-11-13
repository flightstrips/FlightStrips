#pragma once

#include <functional>
#include <memory>
#include <string>
#include <unordered_map>
#include <vector>
#include "TimedEventHandler.h"
#include "websocket/MembershipState.h"

namespace FlightStrips {
    class FlightStripsPlugin;
    namespace websocket { class WebSocketService; }
}

namespace FlightStrips::handlers {
    class AircraftSelectionHandler final : public TimedEventHandler {
    public:
        using Context = websocket::membership_wire::MembershipContext;
        using Reader = std::function<std::string()>;
        using Contexts = std::function<std::vector<Context>()>;
        using Sender = std::function<bool(const std::string&, const Context&)>;

        AircraftSelectionHandler(std::weak_ptr<FlightStripsPlugin> plugin,
                                 std::shared_ptr<websocket::WebSocketService> websocket);
        AircraftSelectionHandler(Reader reader, Contexts contexts, Sender sender);
        void OnTimer(int time) override;

    private:
        Reader m_reader;
        Contexts m_contexts;
        Sender m_sender;
        std::unordered_map<std::string, std::string> m_published;
    };
}
