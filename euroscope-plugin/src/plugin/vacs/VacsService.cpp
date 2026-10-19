#include "VacsService.h"
#include <algorithm>
#include <cctype>
#include <utility>

namespace FlightStrips::vacs {
    VacsService::VacsService(std::shared_ptr<configuration::UserConfig> config, SocketFactory factory)
        : config_(std::move(config)), client_([this](const std::string& message) { if (socket_) socket_->Send(message); }), factory_(std::move(factory)) {
        if (!factory_) factory_ = [](const std::string& url, message_callback receive, on_connected_callback open) {
            return std::make_unique<websocket::WebSocket>(url, std::move(receive), std::move(open), true);
        };
        client_.Close(Enabled());
    }
    VacsService::~VacsService() { Stop(); }
    std::string VacsService::BuildUrl(const std::string& host) {
        if (host.empty()) return "ws://localhost:9600/ws";
        if (host.size() > 253 || host.find_first_not_of("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.-:[]") != std::string::npos) return {};
        std::string address = host;
        if (address.find(':') != std::string::npos) {
            if (address.front() == '[' && address.back() == ']') address = address.substr(1, address.size() - 2);
            IN6_ADDR parsed{};
            if (InetPtonA(AF_INET6, address.c_str(), &parsed) != 1) return {};
            address = "[" + address + "]";
        } else if (address.find_first_of("[]") != std::string::npos) {
            return {};
        }
        return "ws://" + address + ":9600/ws";
    }
    void VacsService::Enqueue(Event event) {
        std::lock_guard lock(mutex_);
        events_.push_back(std::move(event));
    }
    void VacsService::Stop() {
        stopped_ = true;
        ++generation_;
        socket_.reset();
        std::lock_guard lock(mutex_);
        events_.clear(); picker_.clear();
        client_.Close(false);
    }
    void VacsService::ToggleEnabled() {
        const bool enabled = !Enabled();
        Stop();
        config_->SetVacsEnabled(enabled);
        stopped_ = false; backoff_ = 1; retryAt_ = {};
        client_.Close(enabled);
        RefreshScreens();
    }
    void VacsService::ToggleMini() { config_->SetVacsShowInMinimized(!MiniPreference()); RefreshScreens(); }
    void VacsService::AttachScreen(void* screen, std::function<void()> refresh) { screens_[screen] = std::move(refresh); }
    void VacsService::DetachScreen(void* screen) { screens_.erase(screen); }
    void VacsService::RefreshScreens() { for (const auto& [screen, refresh] : screens_) refresh(); }
    void VacsService::Tick(Time now) {
        now_ = now;
        if (stopped_ || !Enabled()) return;
        std::vector<Event> events;
        { std::lock_guard lock(mutex_); events.swap(events_); }
        for (const auto& event : events) {
            if (event.generation != generation_) continue;
            if (event.open) { client_.Open(now); backoff_ = 1; retryAt_ = {}; }
            else client_.Receive(event.payload, now);
        }
        if (socket_) {
            const auto status = socket_->GetStatus();
            if (status == websocket::WEBSOCKET_STATUS_DISCONNECTED || status == websocket::WEBSOCKET_STATUS_FAILED) {
                ++generation_;
                socket_.reset();
                retryAt_ = now + std::chrono::seconds(backoff_);
                client_.Close(true, backoff_);
                backoff_ = std::min(30, backoff_ * 2);
                picker_.clear();
            }
        }
        if (!socket_ && now >= retryAt_) {
            const auto url = BuildUrl(config_->GetVacsHost());
            if (url.empty()) {
                if (retryAt_ == Time{}) error_ = "Invalid VACS host. Set [vacs] host to a hostname or IP, without scheme, port or path.";
                retryAt_ = now + std::chrono::seconds(30);
                return;
            }
            const auto generation = ++generation_;
            socket_ = factory_(url,
                [this, generation](const std::string& message) { Enqueue({generation, false, message}); },
                [this, generation] { Enqueue({generation, true, {}}); });
            socket_->Connect();
        }
        client_.Tick(now);
        RefreshScreens();
    }
    View VacsService::GetView() const {
        auto view = client_.GetView();
        if (view.state == State::Reconnecting && retryAt_ != Time{}) {
            const auto seconds = std::max<long long>(0, std::chrono::duration_cast<std::chrono::milliseconds>(retryAt_ - now_).count() + 999) / 1000;
            view.label = "Retry in " + std::to_string(seconds) + "s";
        }
        return view;
    }
    bool VacsService::Execute(Action action, const std::string& callId) {
        const auto result = client_.Execute(action, callId, std::chrono::steady_clock::now());
        RefreshScreens();
        return result;
    }
    std::vector<PickerRow> VacsService::OpenPicker() {
        picker_.clear();
        const auto view = GetView();
        if (view.state != State::Idle || !view.actionable) return {};
        const auto clients = client_.GetClients();
        for (const auto& client : clients) {
            auto label = client.position.empty() ? (client.name.empty() ? client.id : client.name) : client.position;
            if (std::count_if(clients.begin(), clients.end(), [&](const Client& other) { return (other.position.empty() ? (other.name.empty() ? other.id : other.name) : other.position) == label; }) > 1) label += " [" + client.id + "]";
            picker_.push_back({label, client.frequency, client.id});
        }
        return picker_;
    }
    void VacsService::Select(const std::string& label) {
        const auto found = std::find_if(picker_.begin(), picker_.end(), [&](const PickerRow& row) { return row.label == label; });
        if (found == picker_.end()) return;
        const auto target = found->id;
        picker_.clear();
        if (!client_.Execute(Action::Call, {}, std::chrono::steady_clock::now(), target)) error_ = "VACS controller is no longer available or call state changed. Open Call again.";
        RefreshScreens();
    }
    std::string VacsService::TakeError() {
        if (!error_.empty()) return std::exchange(error_, {});
        return client_.TakeError();
    }
}
