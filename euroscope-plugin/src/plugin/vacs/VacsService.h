#pragma once

#include "VacsClient.h"
#include "configuration/UserConfig.h"
#include "websocket/WebSocket.h"
#include <memory>
#include <mutex>

namespace FlightStrips::vacs {
    struct PickerRow { std::string label, frequency, id; };
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
        bool Execute(Action action, const std::string& callId);
        std::vector<PickerRow> OpenPicker();
        void Select(const std::string& label);
        std::string TakeError();
        static std::string BuildUrl(const std::string& host);
    private:
        struct Event { unsigned long long generation; bool open; std::string payload; };
        std::shared_ptr<configuration::UserConfig> config_;
        VacsClient client_;
        std::unique_ptr<websocket::WebSocket> socket_;
        std::mutex mutex_;
        std::vector<Event> events_;
        std::vector<PickerRow> picker_;
        unsigned long long generation_ = 0;
        int backoff_ = 1;
        Time retryAt_{};
        Time now_{};
        bool stopped_ = false;
        std::string error_;
        SocketFactory factory_;
        std::map<void*, std::function<void()>> screens_;
        void Enqueue(Event event);
        void RefreshScreens();
    };
}
