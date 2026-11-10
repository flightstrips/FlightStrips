#include "AircraftSelectionHandler.h"
#include <unordered_set>
#include "plugin/FlightStripsPlugin.h"
#include "websocket/WebSocketService.h"

namespace FlightStrips::handlers {
    AircraftSelectionHandler::AircraftSelectionHandler(
        std::weak_ptr<FlightStripsPlugin> plugin,
        std::shared_ptr<websocket::WebSocketService> websocket)
        : AircraftSelectionHandler(
            [plugin] {
                const auto owner = plugin.lock();
                if (!owner) return std::string{};
                const auto selected = owner->FlightPlanSelectASEL();
                return selected.IsValid() ? std::string(selected.GetCallsign()) : std::string{};
            },
            [websocket] {
                std::vector<Context> contexts;
                if (websocket->IsConnected()) {
                    for (const auto& [id, member] : websocket->Memberships().members) {
                        if (member.role != websocket::STATE_UNKNOWN) contexts.push_back(member.context);
                    }
                }
                return contexts;
            },
            [websocket](const std::string& callsign, const Context& context) {
                if (!websocket->IsConnected()) return false;
                websocket->SendCaptured(AircraftSelectionEvent(callsign), context);
                return true;
            }) {}

    AircraftSelectionHandler::AircraftSelectionHandler(Reader reader, Contexts contexts, Sender sender)
        : m_reader(std::move(reader)), m_contexts(std::move(contexts)), m_sender(std::move(sender)) {}

    void AircraftSelectionHandler::OnTimer(int) {
        const auto contexts = m_contexts();
        // Read ASEL once per tick, never once per membership or aircraft.
        const auto callsign = m_reader();
        std::unordered_set<std::string> active;
        for (const auto& context : contexts) {
            auto identity = context;
            identity.set_master_epoch(0);
            const auto key = identity.SerializeAsString();
            active.insert(key);
            const auto previous = m_published.find(key);
            if ((previous == m_published.end() || previous->second != callsign) && m_sender(callsign, context)) {
                m_published[key] = callsign;
            }
        }
        std::erase_if(m_published, [&](const auto& entry) { return !active.contains(entry.first); });
    }
}
