#pragma once
#include "VacsService.h"
#include "handlers/MessageHandler.h"
#include "websocket/WebSocketService.h"

namespace FlightStrips::vacs {
    // Runs on the SDK callback thread, alongside the shared native VACS client.
    class VacsRelay final : public handlers::MessageHandler {
    public:
        VacsRelay(std::shared_ptr<VacsService> service, std::weak_ptr<websocket::WebSocketService> transport)
            : service_(std::move(service)), transport_(std::move(transport)) {}
        void OnMessages(const std::vector<std::string>& messages) override;
        static websocket::protobuf::wire::VacsState Snapshot(const VacsService& service, uint64_t sequence);
    private:
        std::shared_ptr<VacsService> service_;
        std::weak_ptr<websocket::WebSocketService> transport_;
        uint64_t sequence_ = 0;
    };
}
