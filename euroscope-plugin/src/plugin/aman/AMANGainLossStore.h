#pragma once

#include <atomic>
#include <memory>
#include <optional>
#include <string>
#include <unordered_map>

#include "handlers/MessageHandler.h"
#include "handlers/ConnectionEventHandler.h"
#include "websocket/generated/proto/euroscope.pb.h"

namespace FlightStrips::aman {
    struct GainLossValue {
        std::string flightId;
        std::string callsign;
        std::optional<long long> seconds;
        std::optional<std::string> referencePoint;
        std::optional<std::string> targetTime;
        std::optional<std::string> predictedTime;
        std::string dataStatus;
    };

    struct GainLossSnapshot {
        unsigned long long revision = 0;
        bool hasRevision = false;
        int version = 0;
        std::string airport;
        std::string generatedAt;
        bool authoritative = false;
        std::unordered_map<std::string, GainLossValue> byFlightId;
        std::unordered_map<std::string, std::string> flightIdByCallsign;
    };

    class AMANGainLossStore final : public handlers::MessageHandler, public handlers::ConnectionEventHandler {
    public:
        AMANGainLossStore();

        void OnMessages(const std::vector<std::string>& messages) override;
        void Online() override;
        [[nodiscard]] std::shared_ptr<const GainLossSnapshot> Snapshot() const;
        [[nodiscard]] std::optional<GainLossValue> FindByCallsign(const std::string& callsign) const;

    private:
        std::atomic<std::shared_ptr<const GainLossSnapshot>> snapshot_;

        static std::string NormalizeCallsign(std::string callsign);
        static std::shared_ptr<const GainLossSnapshot> Parse(
            const flightstrips::euroscope::v1::AMANGainLossEvent& message);
        void Clear();
    };
}
