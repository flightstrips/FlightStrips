#include "VacsRelay.h"

namespace FlightStrips::vacs {
    namespace wire = websocket::protobuf::wire;

    wire::VacsState VacsRelay::Snapshot(const VacsService& service, uint64_t sequence) {
        const auto view = service.GetView();
        wire::VacsState state;
        switch (view.state) {
            case State::Disabled: state.set_status("disabled"); break;
            case State::Unavailable: state.set_status("unavailable"); break;
            case State::Reconnecting: state.set_status("reconnecting"); break;
            case State::Unauthenticated: state.set_status("unauthenticated"); break;
            case State::Disconnected: state.set_status("disconnected"); break;
            case State::Ambiguous: state.set_status("ambiguous"); break;
            case State::Synchronizing: state.set_status("synchronizing"); break;
            case State::Idle: state.set_status("idle"); break;
            case State::Incoming: state.set_status("incoming"); break;
            case State::Outgoing: state.set_status("outgoing"); break;
            case State::Connecting: state.set_status("connecting"); break;
            case State::Connected: state.set_status("connected"); break;
        }
        state.set_label(view.label);
        state.set_tooltip(view.state == State::Disabled ? "Switch VACS On in the FlightStrips EuroScope panel." : view.tooltip);
        state.set_call_id(view.callId);
        state.set_pending(view.pending);
        state.set_actionable(view.actionable);
        state.set_waiting(static_cast<uint32_t>(view.waiting));
        state.set_own_client_id(service.OwnClientId());
        state.set_own_position_id(service.OwnPositionId());
        state.set_sequence(sequence);
        for (const auto& client : service.GetClients()) {
            auto* peer = state.add_clients();
            peer->set_id(client.id);
            peer->set_display_name(client.name);
            peer->set_frequency(client.frequency);
            peer->set_position_id(client.position);
        }
        return state;
    }

    void VacsRelay::OnMessages(const std::vector<std::string>& messages) {
        const auto transport = transport_.lock();
        if (!transport) return;
        for (const auto& bytes : messages) {
            wire::Envelope envelope;
            if (!websocket::protobuf::ParseEnvelope(bytes, envelope) || !envelope.has_vacs_request()) continue;
            // MembershipState has already fenced this frame before dispatch.
            if (envelope.command_origin() != wire::COMMAND_ORIGIN_HUMAN) continue;
            const auto& request = envelope.vacs_request();
            if (request.request_id().empty() || request.request_id().size() > 64) continue;
            wire::VacsResultEvent result;
            result.set_request_id(request.request_id());
            bool accepted = request.action() == "status" && request.call_id().empty() && request.client_id().empty();
            if (!accepted) {
                std::optional<Action> action;
                if (request.action() == "dial") action = Action::Call;
                else if (request.action() == "answer") action = Action::Answer;
                else if (request.action() == "reject") action = Action::Reject;
                else if (request.action() == "cancel") action = Action::Cancel;
                else if (request.action() == "end") action = Action::End;
                if (action && service_->Enabled()) accepted = service_->Execute(*action, request.call_id(), request.client_id());
                if (!accepted) result.set_error("VACS call state changed or controls are unavailable. Check the current status before trying again.");
            }
            *result.mutable_state() = Snapshot(*service_, ++sequence_);
            transport->SendVacsResult(result, envelope.context());
        }
    }
}
