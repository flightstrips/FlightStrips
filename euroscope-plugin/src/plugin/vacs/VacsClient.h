#pragma once

#include <chrono>
#include <functional>
#include <map>
#include <set>
#include <string>
#include <vector>
#include <nlohmann/json.hpp>

namespace FlightStrips::vacs {
    using Json = nlohmann::json;
    using Time = std::chrono::steady_clock::time_point;
    enum class State { Disabled, Unavailable, Reconnecting, Unauthenticated, Disconnected, Ambiguous, Synchronizing, Idle, Incoming, Outgoing, Connecting, Connected };
    enum class Action { Call, Answer, Reject, Cancel, End };
    struct Client { std::string id, name, frequency, position; };
    struct Call { std::string id, peer; };
    struct View {
        State state = State::Disabled;
        std::string label = "Disabled", mini = "Off", tooltip, callId;
        bool pending = false, actionable = false;
        std::size_t waiting = 0;
    };
    class VacsClient {
    public:
        explicit VacsClient(std::function<void(const std::string&)> send);
        void Open(Time now);
        void Close(bool enabled, int retrySeconds = 0);
        void Receive(const std::string& raw, Time now);
        void Tick(Time now);
        bool Execute(Action action, const std::string& expectedCallId, Time now, const std::string& clientId = {});
        [[nodiscard]] View GetView() const;
        [[nodiscard]] std::vector<Client> GetClients() const;
        std::string TakeError();
    private:
        struct Request {
            std::string command, callId, peer;
            Time deadline;
            unsigned long long revision;
        };
        std::function<void(const std::string&)> send_;
        std::map<std::string, Request> requests_;
        std::set<std::string> terminated_;
        std::vector<Client> clients_;
        std::vector<Call> incoming_;
        std::string ownId_, position_, station_, callId_, peer_, error_, pendingId_, snapshotId_, lastError_;
        bool enabled_ = false, online_ = false, authenticated_ = false, signaling_ = false, ambiguous_ = false;
        bool snapshot_ = false, store_ = false, uncertain_ = false;
        State callState_ = State::Idle;
        unsigned long long nextId_ = 0, revision_ = 0;
        int retrySeconds_ = 0;
        Time pingAt_{};
        std::string Invoke(const std::string& command, const Json& args, Time now, const std::string& callId = {}, const std::string& peer = {});
        void Sync(Time now);
        void Snapshot(const Json& data);
        void Event(const std::string& name, const Json& payload, Time now);
        void Store(const Json& payload);
        void Finish(const std::string& callId);
        void Display(const std::string& callId, const std::string& peer, State state);
        std::string PeerName(const std::string& id) const;
        void SetClients(const Json& clients);
    };
}
