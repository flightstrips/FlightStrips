#pragma once
#include <chrono>
#include <deque>
#include <functional>
#include <optional>
#include <string>
#include <vector>

namespace FlightStrips::websocket {
    // The existing two-minute radar-target wait retains the exact authority
    // captured by its typed generate_squawk message. No SDK work occurs here.
    class PendingSquawkQueue {
        struct Request {
            std::string callsign;
            std::chrono::steady_clock::time_point requestedAt;
            std::function<bool()> valid,ready;
            std::function<bool(const std::function<void()>&)> execute;
        };
        std::deque<Request> pending_;
        std::optional<Request> selected_;
    public:
        void Add(std::string callsign,std::function<bool()> valid,std::function<bool()> ready,std::function<bool(const std::function<void()>&)> execute) {
            pending_.push_back({std::move(callsign),std::chrono::steady_clock::now(),std::move(valid),std::move(ready),std::move(execute)});
        }
        std::vector<std::string> Expire() {
            std::vector<std::string> expired;
            for(auto it=pending_.begin();it!=pending_.end();) {
                if(std::chrono::steady_clock::now()-it->requestedAt>=std::chrono::minutes(2) || !it->valid()) {expired.push_back(it->callsign);it=pending_.erase(it);} else ++it;
            }
            return expired;
        }
        std::optional<std::string> TakeReady(const std::function<bool(const std::string&)>& visible) {
            Expire();
            for(auto it=pending_.begin();it!=pending_.end();++it) {
                if(!it->ready() || !visible(it->callsign)) continue;
                selected_=std::move(*it);pending_.erase(it);return selected_->callsign;
            }
            return {};
        }
        bool Execute(const std::string& callsign,const std::function<void()>& action) {
            if(!selected_ || selected_->callsign!=callsign) return false;
            auto request=std::move(*selected_);selected_.reset();
            return request.valid() && request.ready() && request.execute(action);
        }
    };
}
