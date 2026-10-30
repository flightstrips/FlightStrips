#pragma once

#include "VacsClient.h"
#include "configuration/UserConfig.h"
#include "websocket/WebSocket.h"
#include <condition_variable>
#include <deque>
#include <memory>
#include <mutex>
#include <thread>

namespace FlightStrips::vacs {
    struct PickerRow { std::string label, frequency, id; };
    // Destroys retired sockets off the EuroScope thread: WebSocket teardown joins ASIO's resolver worker,
    // which cannot be interrupted while a getaddrinfo lookup is in flight.
    class SocketReaper {
    public:
        SocketReaper();
        ~SocketReaper();
        void Retire(std::unique_ptr<websocket::WebSocket> socket);
    private:
        std::mutex mutex_;
        std::condition_variable wake_;
        std::deque<std::unique_ptr<websocket::WebSocket>> sockets_;
        bool stopping_ = false;
        std::thread thread_;
    };
    class VacsService {
    public:
        using SocketFactory = std::function<std::unique_ptr<websocket::WebSocket>(const std::string&, message_callback, on_connected_callback)>;
        explicit VacsService(std::shared_ptr<configuration::UserConfig> config, SocketFactory factory = {});
        ~VacsService();
        void Tick(Time now = std::chrono::steady_clock::now());
        void Stop();
        void ToggleEnabled();
        void ToggleMini();
        void AttachScreen(void* screen, std::function<void()> refresh);
        void DetachScreen(void* screen);
        [[nodiscard]] bool Enabled() const { return config_->GetVacsEnabled(); }
        [[nodiscard]] bool ShowMini() const { return Enabled() && config_->GetVacsShowInMinimized(); }
        [[nodiscard]] bool MiniPreference() const { return config_->GetVacsShowInMinimized(); }
        [[nodiscard]] View GetView() const;
        [[nodiscard]] std::size_t QueuedEvents() const;
        bool Execute(Action action, const std::string& callId);
        std::vector<PickerRow> OpenPicker();
        void Select(const std::string& label);
        std::string TakeError();
        static std::string BuildUrl(const std::string& host);
    private:
        static constexpr std::size_t kMaxEventsPerTick = 64;
        struct Event { unsigned long long generation; bool open; std::string payload; };
        // Socket callbacks hold the inbox, not the service, so a retiring socket can still deliver safely.
        struct Inbox { std::mutex mutex; std::deque<Event> events; };
        std::shared_ptr<configuration::UserConfig> config_;
        VacsClient client_;
        std::shared_ptr<Inbox> inbox_ = std::make_shared<Inbox>();
        SocketReaper reaper_;
        std::unique_ptr<websocket::WebSocket> socket_;
        std::vector<PickerRow> picker_;
        unsigned long long generation_ = 0;
        int backoff_ = 1;
        Time retryAt_{};
        Time now_{};
        bool stopped_ = false;
        std::string error_;
        SocketFactory factory_;
        std::map<void*, std::function<void()>> screens_;
        void RetireSocket();
        void RefreshScreens();
    };
}
