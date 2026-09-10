#include "AMANGainLossStore.h"

#include <algorithm>
#include <cctype>
#include <stdexcept>

namespace FlightStrips::aman {
    namespace {
        auto RequiredTrimmedString(std::string value, const char* field) -> std::string {
            const auto first = value.find_first_not_of(" \t\r\n");
            if (first == std::string::npos) throw std::invalid_argument(field);
            const auto last = value.find_last_not_of(" \t\r\n");
            if (first != 0 || last + 1 != value.size()) throw std::invalid_argument(field);
            return value;
        }
    }

    AMANGainLossStore::AMANGainLossStore()
        : snapshot_(std::make_shared<const GainLossSnapshot>()) {}

    void AMANGainLossStore::OnMessages(const std::vector<std::string>& messages) {
        for (const auto& bytes : messages) {
            flightstrips::euroscope::v1::Envelope envelope;
            if (!envelope.ParseFromString(bytes) ||
                envelope.event_case() != flightstrips::euroscope::v1::Envelope::kAmanGainLoss) continue;
            try {
                auto replacement = Parse(envelope.aman_gain_loss());
                const auto current = Snapshot();
                if (!current->hasRevision || replacement->revision > current->revision ||
                    (replacement->revision == current->revision && replacement->authoritative != current->authoritative)) {
                    snapshot_.store(std::move(replacement));
                }
            } catch (...) {
                Clear();
            }
        }
    }

    auto AMANGainLossStore::Snapshot() const -> std::shared_ptr<const GainLossSnapshot> {
        return snapshot_.load();
    }

    void AMANGainLossStore::Online() {
        // A reconnect starts a new revision stream. Hide the previous
        // connection's values until this connection receives its replacement.
        snapshot_.store(std::make_shared<const GainLossSnapshot>());
    }

    auto AMANGainLossStore::FindByCallsign(const std::string& callsign) const -> std::optional<GainLossValue> {
        const auto snapshot = Snapshot();
        const auto callsignEntry = snapshot->flightIdByCallsign.find(NormalizeCallsign(callsign));
        if (callsignEntry == snapshot->flightIdByCallsign.end()) return std::nullopt;
        const auto value = snapshot->byFlightId.find(callsignEntry->second);
        if (value == snapshot->byFlightId.end()) return std::nullopt;
        return value->second;
    }

    auto AMANGainLossStore::NormalizeCallsign(std::string callsign) -> std::string {
        const auto first = callsign.find_first_not_of(" \t\r\n");
        if (first == std::string::npos) return "";
        callsign.erase(0, first);
        callsign.erase(callsign.find_last_not_of(" \t\r\n") + 1);
        std::ranges::transform(callsign, callsign.begin(), [](const unsigned char value) {
            return static_cast<char>(std::toupper(value));
        });
        return callsign;
    }

    auto AMANGainLossStore::Parse(const flightstrips::euroscope::v1::AMANGainLossEvent& message)
        -> std::shared_ptr<const GainLossSnapshot> {
        if (message.version() != 1) {
            throw std::invalid_argument("invalid AMAN gain/loss envelope");
        }

        auto result = std::make_shared<GainLossSnapshot>();
        result->version = message.version();
        result->revision = message.revision();
        result->hasRevision = true;
        result->airport = RequiredTrimmedString(message.airport(), "airport");
        result->generatedAt = RequiredTrimmedString(message.generated_at(), "generated_at");
        result->authoritative = message.authoritative();

        for (const auto& item : message.values()) {
            GainLossValue value;
            value.flightId = RequiredTrimmedString(item.flight_id(), "flight_id");
            value.callsign = NormalizeCallsign(RequiredTrimmedString(item.callsign(), "callsign"));
            value.dataStatus = RequiredTrimmedString(item.data_status(), "data_status");
            if (value.dataStatus != "fresh" && value.dataStatus != "stale" && value.dataStatus != "disconnected") {
                throw std::invalid_argument("invalid AMAN data status");
            }
            const auto hasPresentation = item.has_gain_loss_seconds();
            if (hasPresentation != item.has_reference_point() || hasPresentation != item.has_target_time() ||
                hasPresentation != item.has_predicted_time()) {
                throw std::invalid_argument("partial AMAN presentation value");
            }
            if (hasPresentation) {
                value.seconds = item.gain_loss_seconds();
                value.referencePoint = RequiredTrimmedString(item.reference_point(), "reference_point");
                value.targetTime = RequiredTrimmedString(item.target_time(), "target_time");
                value.predictedTime = RequiredTrimmedString(item.predicted_time(), "predicted_time");
            }
            if (!result->byFlightId.emplace(value.flightId, value).second ||
                !result->flightIdByCallsign.emplace(value.callsign, value.flightId).second) {
                throw std::invalid_argument("duplicate AMAN flight identity");
            }
        }
        return result;
    }

    void AMANGainLossStore::Clear() {
        const auto current = Snapshot();
        auto empty = std::make_shared<GainLossSnapshot>();
        empty->revision = current->revision;
        empty->hasRevision = current->hasRevision;
        empty->authoritative = current->authoritative;
        snapshot_.store(std::move(empty));
    }
}
