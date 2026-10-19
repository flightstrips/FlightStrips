#include "VacsClient.h"
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
        std::string Short(const std::string& name) {
            const auto split = name.find_last_of('_');
            return split == std::string::npos ? name : name.substr(split + 1);
        }
    }
    VacsClient::VacsClient(std::function<void(const std::string&)> send) : send_(std::move(send)) {}
    void VacsClient::Close(bool enabled, int retrySeconds) {
        enabled_ = enabled;
        online_ = authenticated_ = signaling_ = ambiguous_ = snapshot_ = store_ = uncertain_ = false;
        requests_.clear(); incoming_.clear(); clients_.clear(); terminated_.clear();
        ownId_.clear(); position_.clear(); station_.clear(); callId_.clear(); peer_.clear(); pendingId_.clear();
        callState_ = State::Idle;
        retrySeconds_ = retrySeconds;
        lastError_.clear();
        ++revision_;
    }
    std::string VacsClient::Invoke(const std::string& command, const Json& args, Time now, const std::string& callId, const std::string& peer) {
        const auto id = "fs-" + std::to_string(++nextId_);
        requests_[id] = {command, callId, peer, now + std::chrono::seconds(10), revision_};
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
             "signaling:call-invite", "signaling:add-incoming-to-call-list", "signaling:accept-incoming-call", "signaling:outgoing-call-accepted",
             "signaling:call-end", "signaling:call-reject", "signaling:force-call-end", "webrtc:call-connected", "webrtc:call-disconnected",
             "webrtc:call-error", "store:sync", "error"}) {
            send_(Json{{"type", "subscribe"}, {"event", name}}.dump());
        }
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
        callId_ = callId; peer_ = peer; callState_ = state;
        std::erase_if(incoming_, [&](const Call& call) { return call.id == callId; });
    }
    void VacsClient::Finish(const std::string& callId) {
        if (callId.empty()) return;
        const auto finished = callId;
        terminated_.insert(finished);
        std::erase_if(incoming_, [&](const Call& call) { return call.id == finished; });
        if (callId_ == finished) { callId_.clear(); peer_.clear(); callState_ = State::Idle; }
    }
    void VacsClient::Snapshot(const Json& data) {
        if (!data.is_object() || !data.contains("connectionState") || !data.contains("clientId") || !data.contains("incomingCalls") || !data["incomingCalls"].is_array()) {
            error_ = "VACS 2.8 compatibility error: invalid session snapshot."; return;
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
        incoming_.clear();
        for (const auto& call : data["incomingCalls"]) {
            const auto id = Text(call, "callId");
            if (!id.empty() && !terminated_.contains(id) && id != callId_ && std::none_of(incoming_.begin(), incoming_.end(), [&](const Call& item) { return item.id == id; })) incoming_.push_back({id, Text(call.value("source", Json::object()), "clientId")});
        }
        if (data.contains("outgoingCall") && data["outgoingCall"].is_object() && callId_.empty()) {
            const auto& call = data["outgoingCall"];
            Display(Text(call, "callId"), Target(call.value("target", Json::object())), State::Outgoing);
        }
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
        if (state.contains("callDisplay") && state["callDisplay"].is_null()) return;
        if (!state.contains("callDisplay")) {
            if (!callId_.empty()) Finish(callId_);
            store_ = true; uncertain_ = false; return;
        }
        const auto& display = state["callDisplay"];
        if (!display.is_object() || !display.contains("call") || !display["call"].is_object()) return;
        const auto& call = display["call"];
        const auto id = Text(call, "callId");
        const auto type = Text(display, "type");
        if (id.empty() || (type != "accepted" && type != "outgoing" && type != "rejected" && type != "error")) return;
        if (type == "error" || type == "rejected") Finish(id);
        else {
            auto peer = Text(display, "targetClientId");
            const bool accepted = type == "accepted" || !peer.empty();
            if (peer.empty() && type == "outgoing") peer = Target(call.value("target", Json{}));
            if (peer.empty()) peer = Text(call.value("source", Json{}), "clientId");
            const auto connection = Text(display, "connectionState");
            Display(id, peer, connection == "connected" || connection == "degraded" ? State::Connected : accepted ? State::Connecting : State::Outgoing);
        }
        store_ = true; uncertain_ = false;
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
        else if (name == "signaling:call-invite" || name == "signaling:add-incoming-to-call-list") {
            const auto id = Id(payload);
            if (!id.empty() && !terminated_.contains(id) && id != callId_ && std::none_of(incoming_.begin(), incoming_.end(), [&](const Call& call) { return call.id == id; })) incoming_.push_back({id, Text(payload.value("source", Json{}), "clientId")});
        }
        else if (name == "signaling:call-end" || name == "signaling:call-reject" || name == "signaling:force-call-end" || name == "webrtc:call-disconnected" || name == "webrtc:call-error") {
            Finish(Id(payload));
            if (name == "webrtc:call-error") error_ = "VACS call failed: " + Text(payload, "reason");
        }
        else if (name == "signaling:outgoing-call-accepted" || name == "signaling:accept-incoming-call") {
            const auto id = Id(payload);
            auto peer = peer_;
            if (!Text(payload, "acceptingClientId").empty()) peer = Text(payload, "acceptingClientId");
            for (const auto& call : incoming_) if (call.id == id) peer = call.peer;
            if (id == callId_ || std::any_of(incoming_.begin(), incoming_.end(), [&](const Call& call) { return call.id == id; })) Display(id, peer, State::Connecting);
        }
        else if (name == "webrtc:call-connected") {
            if (Id(payload) == callId_) callState_ = State::Connected;
            else Sync(now);
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
                error_ = "VACS " + request.command + " failed: " + Text(message.value("error", Json{}), "detail");
                if (request.command == "app_frontend_ready" || request.command == "remote_get_session_state" || request.command == "remote_request_store_sync") { snapshot_ = false; error_ += " Check VACS 2.8 remote-control compatibility."; }
                return;
            }
            const auto data = message.value("data", Json{});
            if (request.command == "app_frontend_ready") Sync(now);
            else if (request.command == "remote_get_session_state") {
                if (request.revision != revision_) { Invoke("remote_get_session_state", Json::object(), now); return; }
                Snapshot(data);
                if (snapshot_) Invoke("remote_request_store_sync", Json::object(), now);
            }
            else if (request.command == "signaling_start_call") {
                const auto callId = Id(data);
                if (callId.empty()) {
                    error_ = "VACS returned an invalid dial result. Resynchronizing; no call retried.";
                    uncertain_ = true; Sync(now);
                }
                else if (request.revision == revision_) Display(callId, request.peer, State::Outgoing);
                else if (!terminated_.contains(callId) && callId_.empty()) { uncertain_ = true; Sync(now); }
            }
            else if (request.command == "signaling_accept_call") {
                if (callState_ != State::Connected || callId_ != request.callId) Display(request.callId, request.peer, State::Connecting);
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
            pendingId_ = Invoke("signaling_start_call", {{"target", {{"client", clientId}}}, {"source", source}, {"prio", false}}, now, {}, clientId);
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
            case State::Unavailable: view.label = "VACS offline"; view.mini = "Offline"; view.tooltip = "Start VACS 2.8 and enable remote control on port 9600."; break;
            case State::Reconnecting: view.label = "Retry in " + std::to_string(retrySeconds_) + "s"; view.mini = "Retry"; view.tooltip = "Reconnecting to VACS remote control. No calls are retried."; break;
            case State::Unauthenticated: view.label = "Sign in to VACS"; view.mini = "Sign in"; view.tooltip = "Complete sign-in inside VACS."; break;
            case State::Disconnected: view.label = "Connect position"; view.mini = "Position"; view.tooltip = "Connect your controller position in VACS."; break;
            case State::Ambiguous: view.label = "Select position"; view.mini = "Select"; view.tooltip = "Resolve position selection inside VACS."; break;
            case State::Synchronizing: view.label = "Synchronizing"; view.mini = "Sync"; view.tooltip = "Waiting for VACS session and desktop call-store recovery."; break;
            case State::Idle: view.label = "Ready"; view.mini = "VACS"; view.tooltip = "Call a VACS controller."; break;
            case State::Incoming: view.label = PeerName(incoming_.front().peer) + (incoming_.size() > 1 ? " (" + std::to_string(incoming_.size()) + ")" : ""); view.mini = Short(PeerName(incoming_.front().peer)) + (incoming_.size() > 1 ? " " + std::to_string(incoming_.size()) : ""); break;
            case State::Outgoing: view.label = "Calling " + PeerName(peer_); view.mini = Short(PeerName(peer_)); break;
            case State::Connecting: view.label = "Connecting " + Short(PeerName(peer_)); view.mini = Short(PeerName(peer_)); break;
            case State::Connected: view.label = PeerName(peer_); view.mini = view.label; break;
        }
        if (view.tooltip.empty()) {
            const auto fullPeer = view.state == State::Incoming ? PeerName(incoming_.front().peer) : PeerName(peer_);
            const auto description = view.state == State::Incoming ? "Incoming from " : view.state == State::Outgoing ? "Calling " : view.state == State::Connecting ? "Connecting to " : "Connected to ";
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
