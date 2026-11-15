#include "VacsClient.h"
#include "Logger.hpp"
#include <algorithm>
#include <utility>

namespace FlightStrips::vacs {
    namespace {
        std::string Text(const Json& value, const char* key) {
            return value.is_object() && value.contains(key) && value[key].is_string() ? value[key].get<std::string>() : "";
        }
        std::string Id(const Json& value) { return value.is_string() ? value.get<std::string>() : Text(value, "callId"); }
        std::string Target(const Json& value) {
            for (const auto* key : {"client", "position", "station"}) {
                const auto id = Text(value, key);
                if (!id.empty()) return id;
            }
            return {};
        }
    }
    VacsClient::VacsClient(std::function<void(const std::string&)> send) : send_(std::move(send)) {}
    void VacsClient::Close(bool enabled, int retrySeconds) {
        enabled_ = enabled;
        online_ = authenticated_ = signaling_ = ambiguous_ = snapshot_ = store_ = uncertain_ = reconnecting_ = false;
        invitedTargets_ = Json::array(); peerStates_.clear();
        requests_.clear(); incoming_.clear(); clients_.clear(); terminated_.clear();
        ownId_.clear(); position_.clear(); station_.clear(); callId_.clear(); peer_.clear(); pendingId_.clear();
        callState_ = State::Idle;
        retrySeconds_ = retrySeconds;
        lastError_.clear();
        ++revision_;
    }
    std::string VacsClient::Invoke(const std::string& command, const Json& args, Time now, const std::string& callId, const std::string& peer) {
        const auto id = "fs-" + std::to_string(++nextId_);
        requests_[id] = {command, callId, peer, args, now + std::chrono::seconds(10), revision_};
        if (command == "remote_get_session_state") snapshotId_ = id;
        send_(Json{{"type", "invoke"}, {"id", id}, {"cmd", command}, {"args", args}}.dump());
        return id;
    }
    void VacsClient::Open(Time now) {
        Close(true);
        online_ = true;
        pingAt_ = now + std::chrono::seconds(20);
        for (const auto* name : {"auth:authenticated", "auth:unauthenticated", "signaling:connected", "signaling:disconnected",
             "signaling:ambiguous-position", "signaling:client-list", "signaling:client-connected", "signaling:client-disconnected",
             "signaling:call-invitation", "signaling:accept-incoming-call", "signaling:outgoing-call", "signaling:call-update",
             "signaling:call-end", "signaling:call-reject", "signaling:force-call-end", "webrtc:call-connected", "webrtc:call-disconnected",
             "webrtc:call-degraded", "webrtc:call-reconnecting",
             "webrtc:call-error", "store:sync", "error"}) {
            send_(Json{{"type", "subscribe"}, {"event", name}}.dump());
        }
        if (!versionLogged_) Invoke("app_get_version", Json::object(), now);
        Invoke("app_frontend_ready", Json::object(), now);
    }
    void VacsClient::Sync(Time now) {
        snapshot_ = store_ = false;
        Invoke("remote_get_session_state", Json::object(), now);
    }
    void VacsClient::Tick(Time now) {
        if (!online_) return;
        if (now >= pingAt_) {
            send_(Json{{"type", "ping"}}.dump());
            pingAt_ = now + std::chrono::seconds(20);
        }
        bool resync = false;
        for (auto request = requests_.begin(); request != requests_.end();) {
            if (request->second.deadline > now) { ++request; continue; }
            if (request->second.command == "app_get_version") { request = requests_.erase(request); continue; }
            error_ = "VACS command timed out: " + request->second.command + ". Resynchronizing; no call retried.";
            if (request->first == pendingId_) { pendingId_.clear(); uncertain_ = true; resync = true; }
            request = requests_.erase(request);
        }
        if (resync) Sync(now);
    }
    void VacsClient::SetClients(const Json& clients) {
        if (!clients.is_array()) return;
        clients_.clear();
        for (const auto& client : clients) {
            auto id = Text(client, "id");
            if (!id.empty()) clients_.push_back({id, Text(client, "displayName"), Text(client, "frequency"), Text(client, "positionId")});
        }
    }
    std::string VacsClient::PeerName(const std::string& id) const {
        for (const auto& client : clients_) if (client.id == id) return !client.position.empty() ? client.position : client.name.empty() ? id : client.name;
        return id.empty() ? "Unknown" : id;
    }
    void VacsClient::Display(const std::string& callId, const std::string& peer, State state) {
        if (callId.empty() || terminated_.contains(callId)) return;
        callId_ = callId; peer_ = peer; callState_ = state; reconnecting_ = false;
        std::erase_if(incoming_, [&](const Call& call) { return call.id == callId; });
    }
    void VacsClient::Finish(const std::string& callId) {
        if (callId.empty()) return;
        const auto finished = callId;
        terminated_.insert(finished);
        std::erase_if(incoming_, [&](const Call& call) { return call.id == finished; });
        if (callId_ == finished) {
            callId_.clear(); peer_.clear(); callState_ = State::Idle; reconnecting_ = false;
            invitedTargets_ = Json::array(); peerStates_.clear();
        }
    }
    void VacsClient::SetIncoming(const Json& calls) {
        if (!calls.is_array()) return; // A null live sync leaves the event-driven queue intact.
        incoming_.clear();
        for (const auto& call : calls) {
            const auto id = Text(call, "callId");
            if (!id.empty() && !terminated_.contains(id) && id != callId_ && std::none_of(incoming_.begin(), incoming_.end(), [&](const Call& item) { return item.id == id; }))
                incoming_.push_back({id, Text(call.value("source", Json::object()), "clientId")});
        }
    }
    void VacsClient::Outgoing(const Json& payload) {
        const auto id = Id(payload);
        const auto targets = payload.value("targets", Json{});
        if (id.empty() || terminated_.contains(id) || !targets.is_array() || targets.empty()) return;
        if (!callId_.empty() && callId_ != id) return;
        if (callId_.empty()) {
            peerStates_.clear(); invitedTargets_ = Json::array();
            Display(id, Target(targets.front()), State::Outgoing);
        }
        for (const auto& target : targets) {
            if (std::find(invitedTargets_.begin(), invitedTargets_.end(), target) == invitedTargets_.end()) invitedTargets_.push_back(target);
        }
    }
    void VacsClient::UpdateConnection() {
        if (peerStates_.empty()) {
            if (callState_ == State::Connected) callState_ = State::Connecting;
            reconnecting_ = false;
            return;
        }
        if (!peerStates_.contains(peer_)) peer_ = peerStates_.begin()->first;
        const bool connected = std::any_of(peerStates_.begin(), peerStates_.end(), [](const auto& peer) {
            return peer.second == "connected" || peer.second == "degraded";
        });
        callState_ = connected ? State::Connected : State::Connecting;
        reconnecting_ = !connected && std::any_of(peerStates_.begin(), peerStates_.end(), [](const auto& peer) {
            return peer.second == "disconnected" || peer.second == "reconnecting";
        });
    }
    void VacsClient::UpdateCall(const Json& payload) {
        const auto id = Id(payload);
        const auto participants = payload.value("joinedParticipants", Json{});
        const auto targets = payload.value("invitedTargets", Json{});
        if (id.empty() || !participants.is_object() || !targets.is_array()) return;
        // An empty roster is still a live invitation for a ringing recipient.
        if (id != callId_) return;
        invitedTargets_ = targets;
        std::erase_if(peerStates_, [&](const auto& peer) { return !participants.contains(peer.first); });
        for (const auto& [peer, target] : participants.items()) {
            if (peer != ownId_) peerStates_.try_emplace(peer, "connecting");
        }
        if (peerStates_.empty() && targets.empty()) { Finish(id); return; }
        if (!peerStates_.empty()) {
            if (!peerStates_.contains(peer_)) peer_ = peerStates_.begin()->first;
            UpdateConnection();
        }
        else {
            if (!participants.empty() || callState_ != State::Outgoing) callState_ = State::Connecting;
            if (!targets.empty()) peer_ = Target(targets.front());
            reconnecting_ = false;
        }
    }
    void VacsClient::RemoveTargets(const Json& targets) {
        if (!targets.is_array()) return;
        std::erase_if(invitedTargets_.get_ref<Json::array_t&>(), [&](const Json& target) {
            return std::find(targets.begin(), targets.end(), target) != targets.end();
        });
        if (invitedTargets_.empty() && peerStates_.empty()) Finish(callId_);
        else if (peerStates_.empty()) peer_ = Target(invitedTargets_.front());
    }
    void VacsClient::Snapshot(const Json& data) {
        if (!data.is_object() || !data.contains("connectionState") || !data["connectionState"].is_string() || !data.contains("clientId") ||
            (!data["clientId"].is_null() && !data["clientId"].is_string()) || !data.contains("clients") || !data["clients"].is_array()) {
            error_ = "VACS compatibility error: invalid session snapshot."; return;
        }
        ownId_ = Text(data, "clientId"); authenticated_ = !ownId_.empty();
        signaling_ = Text(data, "connectionState") == "connected";
        ambiguous_ = false;
        position_.clear(); station_.clear();
        if (data.contains("sessionInfo") && data["sessionInfo"].is_object()) {
            position_ = Text(data["sessionInfo"].value("client", Json::object()), "positionId");
        }
        if (data.contains("defaultCallSources") && data["defaultCallSources"].is_array() && !data["defaultCallSources"].empty() && data["defaultCallSources"][0].is_string()) station_ = data["defaultCallSources"][0].get<std::string>();
        SetClients(data.value("clients", Json::array()));
        // VACS 3.x restores calls through remote_request_store_sync, not this snapshot.
        snapshot_ = true;
    }
    void VacsClient::Store(const Json& payload) {
        if (!payload.is_object()) return;
        const auto state = payload.value("state", Json{});
        if (!state.is_object()) return;
        if (Text(payload, "store") == "stations") {
            station_ = Text(state, "temporarySource");
            if (station_.empty()) station_ = Text(state, "defaultSource");
            return;
        }
        if (Text(payload, "store") != "call" || !state.contains("prio") || !state["prio"].is_boolean()) return;
        const auto incoming = state.value("incomingCalls", Json{});
        if (!incoming.is_null() && !incoming.is_array()) return;
        if (state.contains("callDisplay") && state["callDisplay"].is_null()) { SetIncoming(incoming); return; }
        if (!state.contains("callDisplay")) {
            if (!callId_.empty()) Finish(callId_);
            SetIncoming(incoming);
            if (incoming.is_array()) { store_ = true; uncertain_ = false; }
            return;
        }
        const auto& display = state["callDisplay"];
        if (!display.is_object() || !display.contains("call") || !display["call"].is_object()) return;
        const auto& call = display["call"];
        const auto id = Text(call, "callId");
        const auto type = Text(display, "type");
        if (id.empty() || (type != "accepted" && type != "outgoing" && type != "rejected" && type != "error")) return;
        const auto participants = call.value("joinedParticipants", Json{});
        const auto targets = call.value("invitedTargets", Json{});
        if (!participants.is_object() || !targets.is_array()) return;
        if (type == "error" || type == "rejected") Finish(id);
        else {
            if (terminated_.contains(id)) return;
            auto peer = type == "outgoing" ? Target(call.value("target", Json{})) : Text(call.value("source", Json{}), "clientId");
            peerStates_.clear();
            for (const auto& [clientId, participant] : participants.items()) {
                if (clientId != ownId_) peerStates_[clientId] = Text(participant, "state");
            }
            if (!peerStates_.empty() && !peerStates_.contains(peer)) peer = peerStates_.begin()->first;
            invitedTargets_ = targets;
            Display(id, peer, type == "accepted" ? State::Connecting : State::Outgoing);
            UpdateConnection();
        }
        SetIncoming(incoming);
        if (incoming.is_array()) { store_ = true; uncertain_ = false; }
    }
    void VacsClient::Event(const std::string& name, const Json& payload, Time now) {
        if (name == "store:sync") { Store(payload); return; }
        if (name == "auth:authenticated") { ownId_ = payload.is_string() ? payload.get<std::string>() : ""; authenticated_ = !ownId_.empty(); Sync(now); }
        else if (name == "auth:unauthenticated" || name == "signaling:disconnected") {
            requests_.clear(); pendingId_.clear(); uncertain_ = false;
            for (const auto& call : incoming_) terminated_.insert(call.id);
            if (name == "auth:unauthenticated") { authenticated_ = false; ownId_.clear(); }
            signaling_ = false; position_.clear(); station_.clear(); incoming_.clear();
            if (!callId_.empty()) Finish(callId_);
        }
        else if (name == "signaling:connected") { signaling_ = true; ambiguous_ = false; position_ = Text(payload.value("client", Json{}), "positionId"); Sync(now); }
        else if (name == "signaling:ambiguous-position") ambiguous_ = true;
        else if (name == "signaling:client-list") SetClients(payload);
        else if (name == "signaling:client-connected") {
            const auto id = Text(payload, "id");
            std::erase_if(clients_, [&](const Client& client) { return client.id == id; });
            if (!id.empty()) clients_.push_back({id, Text(payload, "displayName"), Text(payload, "frequency"), Text(payload, "positionId")});
        }
        else if (name == "signaling:client-disconnected" && payload.is_string()) std::erase_if(clients_, [&](const Client& client) { return client.id == payload.get<std::string>(); });
        else if (name == "signaling:call-invitation") {
            const auto id = Id(payload);
            if (!id.empty() && !terminated_.contains(id) && id != callId_ && std::none_of(incoming_.begin(), incoming_.end(), [&](const Call& call) { return call.id == id; })) incoming_.push_back({id, Text(payload.value("source", Json{}), "clientId")});
        }
        else if (name == "signaling:outgoing-call") Outgoing(payload);
        else if (name == "signaling:call-update") UpdateCall(payload);
        else if (name == "webrtc:call-disconnected" || name == "webrtc:call-reconnecting" || name == "webrtc:call-connected" || name == "webrtc:call-degraded") {
            const auto id = Id(payload), peer = Text(payload, "peerId");
            if (id.empty() || peer.empty() || peer == ownId_ || terminated_.contains(id)) return;
            if (id != callId_) { Sync(now); return; }
            peerStates_[peer] = name.substr(std::string("webrtc:call-").size());
            UpdateConnection();
        }
        else if (name == "signaling:call-end") {
            // A caller's terminal roster arrives in call-update; call-end may only end one invitation.
            if (Id(payload) != callId_ || callState_ != State::Outgoing) Finish(Id(payload));
        }
        else if (name == "signaling:force-call-end") {
            Finish(Id(payload));
        }
        else if (name == "signaling:call-reject") {
            if (Id(payload) != callId_) Finish(Id(payload));
            else RemoveTargets(payload.value("targets", Json{}));
        }
        else if (name == "webrtc:call-error") {
            error_ = "VACS call failed: " + Text(payload, "reason");
            const auto origin = payload.value("origin", Json{});
            if (payload.value("callEnded", false) || Text(origin, "type") == "call" || Id(payload) != callId_) Finish(Id(payload));
            else if (Text(origin, "type") == "targets") RemoveTargets(origin.value("value", Json{}));
            else if (Text(origin, "type") == "client") {
                peerStates_.erase(Text(origin, "value"));
                if (peerStates_.empty() && invitedTargets_.empty()) Finish(Id(payload));
                else UpdateConnection();
            }
        }
        else if (name == "signaling:accept-incoming-call") {
            const auto id = Id(payload);
            auto peer = peer_;
            for (const auto& call : incoming_) if (call.id == id) peer = call.peer;
            if (id == callId_ || std::any_of(incoming_.begin(), incoming_.end(), [&](const Call& call) { return call.id == id; })) {
                if (id != callId_) { peerStates_.clear(); invitedTargets_ = Json::array(); }
                Display(id, peer, State::Connecting);
                UpdateConnection();
            }
        }
        else if (name == "error") error_ = "VACS: " + Text(payload, "detail");
    }
    void VacsClient::Receive(const std::string& raw, Time now) {
        if (!online_) return;
        try {
            const auto message = Json::parse(raw);
            if (!message.is_object()) return;
            if (Text(message, "type") == "event") {
                const auto name = Text(message, "name");
                if (name.rfind("signaling:", 0) == 0 || name.rfind("auth:", 0) == 0 || name.rfind("webrtc:", 0) == 0 || name == "store:sync") ++revision_;
                Event(name, message.value("payload", Json{}), now); return;
            }
            if (Text(message, "type") != "response") return;
            const auto id = Text(message, "id");
            const auto found = requests_.find(id);
            if (found == requests_.end()) return;
            const auto request = found->second;
            requests_.erase(found);
            if (request.command == "remote_get_session_state" && id != snapshotId_) return;
            if (id == pendingId_) pendingId_.clear();
            if (!message.contains("ok") || !message["ok"].is_boolean() || !message["ok"].get<bool>()) {
                if (request.command == "app_get_version") return;
                error_ = "VACS " + request.command + " failed: " + Text(message.value("error", Json{}), "detail");
                if (request.command == "app_frontend_ready" || request.command == "remote_get_session_state" || request.command == "remote_request_store_sync") { snapshot_ = false; error_ += " Check VACS remote-control compatibility."; }
                return;
            }
            const auto data = message.value("data", Json{});
            if (request.command == "app_get_version") {
                if (!versionLogged_ && data.is_string() && !data.get_ref<const std::string&>().empty()) {
                    Logger::Info("VACS version: {}", data.get<std::string>());
                    versionLogged_ = true;
                }
            }
            else if (request.command == "app_frontend_ready") Sync(now);
            else if (request.command == "remote_get_session_state") {
                if (request.revision != revision_) { Invoke("remote_get_session_state", Json::object(), now); return; }
                Snapshot(data);
                if (snapshot_) Invoke("remote_request_store_sync", Json::object(), now);
            }
            else if (request.command == "signaling_invite_to_call") {
                const auto callId = Id(data);
                if (callId.empty()) {
                    error_ = "VACS returned an invalid dial result. Resynchronizing; no call retried.";
                    uncertain_ = true; Sync(now);
                }
                else if (callId_ == callId || terminated_.contains(callId)) return;
                else if (request.revision == revision_) {
                    invitedTargets_ = request.args["targets"];
                    peerStates_.clear();
                    Display(callId, request.peer, State::Outgoing);
                }
                else if (!terminated_.contains(callId) && callId_.empty()) { uncertain_ = true; Sync(now); }
            }
            else if (request.command == "signaling_accept_call") {
                if (terminated_.contains(request.callId)) return;
                if (callId_ != request.callId) { peerStates_.clear(); invitedTargets_ = Json::array(); }
                Display(request.callId, request.peer, State::Connecting);
                UpdateConnection();
            }
            else if (request.command == "signaling_end_call") Finish(request.callId);
        } catch (const Json::exception&) {
            error_ = "VACS sent an invalid remote-control message; ignored.";
        }
    }
    bool VacsClient::Execute(Action action, const std::string& expectedCallId, Time now, const std::string& clientId) {
        const auto view = GetView();
        if (!view.actionable || view.callId != expectedCallId) return false;
        lastError_.clear();
        if (action == Action::Call) {
            if (view.state != State::Idle) return false;
            const auto clients = GetClients();
            if (std::none_of(clients.begin(), clients.end(), [&](const Client& client) { return client.id == clientId; })) return false;
            Json source{{"clientId", ownId_}, {"positionId", position_}};
            if (!station_.empty()) source["stationId"] = station_;
            pendingId_ = Invoke("signaling_invite_to_call", {{"targets", Json::array({{{"client", clientId}}})}, {"source", source}, {"prio", false}}, now, {}, clientId);
        } else {
            if ((action == Action::Answer || action == Action::Reject) && view.state != State::Incoming) return false;
            if (action == Action::Cancel && view.state != State::Outgoing) return false;
            if (action == Action::End && view.state != State::Connecting && view.state != State::Connected) return false;
            const auto peer = view.state == State::Incoming ? incoming_.front().peer : peer_;
            pendingId_ = Invoke(action == Action::Answer ? "signaling_accept_call" : "signaling_end_call", {{"callId", expectedCallId}}, now, expectedCallId, peer);
        }
        return true;
    }
    std::vector<Client> VacsClient::GetClients() const {
        std::vector<Client> result;
        std::set<std::string> seen;
        for (const auto& client : clients_) if (client.id != ownId_ && !client.id.empty() && (position_.empty() || client.position != position_) && seen.insert(client.id).second) result.push_back(client);
        std::sort(result.begin(), result.end(), [](const Client& left, const Client& right) {
            const auto& leftLabel = left.position.empty() ? left.name : left.position;
            const auto& rightLabel = right.position.empty() ? right.name : right.position;
            return leftLabel == rightLabel ? left.id < right.id : leftLabel < rightLabel;
        });
        return result;
    }
    View VacsClient::GetView() const {
        View view;
        view.waiting = incoming_.size(); view.pending = !pendingId_.empty();
        if (!enabled_) view.state = State::Disabled;
        else if (!online_) view.state = retrySeconds_ ? State::Reconnecting : State::Unavailable;
        else if (uncertain_ && !callId_.empty() && authenticated_ && signaling_) { view.state = callState_; view.callId = callId_; }
        else if (!snapshot_) view.state = State::Synchronizing;
        else if (!authenticated_) view.state = State::Unauthenticated;
        else if (ambiguous_) view.state = State::Ambiguous;
        else if (!signaling_ || position_.empty()) view.state = State::Disconnected;
        else if (!store_ || uncertain_) view.state = State::Synchronizing;
        else if (!callId_.empty()) { view.state = callState_; view.callId = callId_; }
        else if (!incoming_.empty()) { view.state = State::Incoming; view.callId = incoming_.front().id; }
        else view.state = State::Idle;
        switch (view.state) {
            case State::Disabled: view.label = "Disabled"; view.mini = "Off"; view.tooltip = "Enable VACS remote call controls. Audio remains in VACS."; break;
            case State::Unavailable: view.label = "VACS offline"; view.mini = "Offline"; view.tooltip = "Start VACS and enable remote control on port 9600."; break;
            case State::Reconnecting: view.label = "Retry in " + std::to_string(retrySeconds_) + "s"; view.mini = "Retry"; view.tooltip = "Reconnecting to VACS remote control. No calls are retried."; break;
            case State::Unauthenticated: view.label = "Sign in to VACS"; view.mini = "Sign in"; view.tooltip = "Complete sign-in inside VACS."; break;
            case State::Disconnected: view.label = "Connect position"; view.mini = "Position"; view.tooltip = "Connect your controller position in VACS."; break;
            case State::Ambiguous: view.label = "Select position"; view.mini = "Select"; view.tooltip = "Resolve position selection inside VACS."; break;
            case State::Synchronizing: view.label = "Synchronizing"; view.mini = "Sync"; view.tooltip = "Waiting for VACS session and desktop call-store recovery."; break;
            case State::Idle: view.label = "Ready"; view.mini = "VACS"; view.tooltip = "Call a VACS controller."; break;
            case State::Incoming: view.label = PeerName(incoming_.front().peer) + (incoming_.size() > 1 ? " (" + std::to_string(incoming_.size()) + ")" : ""); view.mini = view.label; break;
            case State::Outgoing: view.label = PeerName(peer_); view.mini = view.label; break;
            case State::Connecting: view.label = PeerName(peer_); view.mini = view.label; break;
            case State::Connected: view.label = PeerName(peer_); view.mini = view.label; break;
        }
        if (view.tooltip.empty()) {
            const auto fullPeer = view.state == State::Incoming ? PeerName(incoming_.front().peer) : PeerName(peer_);
            const auto description = view.state == State::Incoming ? "Incoming from " : view.state == State::Outgoing ? "Calling " : view.state == State::Connecting ? (reconnecting_ ? "Reconnecting to " : "Connecting to ") : "Connected to ";
            const auto action = view.state == State::Incoming ? "Answer or reject the oldest call." : view.state == State::Outgoing ? "Cancel this outgoing call." : "End this call.";
            view.tooltip = description + fullPeer + "; " + std::to_string(view.waiting) + " waiting. " + action + " Call ID: " + view.callId;
        }
        view.actionable = !view.pending && !uncertain_ && (view.state == State::Idle || view.state == State::Incoming || view.state == State::Outgoing || view.state == State::Connecting || view.state == State::Connected);
        if (view.pending) view.tooltip += " Waiting for command confirmation.";
        if (uncertain_) view.tooltip += " Resynchronizing after uncertain command result; controls disabled.";
        if (!lastError_.empty()) view.tooltip += " " + lastError_;
        return view;
    }
    std::string VacsClient::TakeError() {
        if (!error_.empty()) lastError_ = error_;
        return std::exchange(error_, {});
    }
}
