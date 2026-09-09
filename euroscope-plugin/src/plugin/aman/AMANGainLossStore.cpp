#include "AMANGainLossStore.h"

#include <algorithm>
#include <cctype>
#include <stdexcept>

namespace FlightStrips::aman {
    namespace {
        auto RequiredTrimmedString(const nlohmann::json& object, const char* key) -> std::string {
            if (!object.contains(key) || !object.at(key).is_string()) throw std::invalid_argument(key);
            auto value = object.at(key).get<std::string>();
            const auto first = value.find_first_not_of(" \t\r\n");
            if (first == std::string::npos) throw std::invalid_argument(key);
            const auto last = value.find_last_not_of(" \t\r\n");
            if (first != 0 || last + 1 != value.size()) throw std::invalid_argument(key);
            return value;
        }
    }

    AMANGainLossStore::AMANGainLossStore()
        : snapshot_(std::make_shared<const GainLossSnapshot>()) {}

    void AMANGainLossStore::OnMessages(const std::vector<nlohmann::json>& messages) {
        for (const auto& message : messages) {
            if (!message.is_object() || message.value("type", "") != "aman_gain_loss") continue;
            try {
                auto replacement = Parse(message);
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

    auto AMANGainLossStore::Parse(const nlohmann::json& message) -> std::shared_ptr<const GainLossSnapshot> {
        if (!message.contains("version") || !message.at("version").is_number_integer() || message.at("version").get<int>() != 1 ||
            !message.contains("revision") ||
            !(message.at("revision").is_number_unsigned() || message.at("revision").is_number_integer()) ||
            message.at("revision").get<long long>() < 0 ||
            !message.contains("authoritative") || !message.at("authoritative").is_boolean() ||
            !message.contains("values") || !message.at("values").is_array()) {
            throw std::invalid_argument("invalid AMAN gain/loss envelope");
        }

        auto result = std::make_shared<GainLossSnapshot>();
        result->version = message.at("version").get<int>();
        result->revision = message.at("revision").get<unsigned long long>();
        result->hasRevision = true;
        result->airport = RequiredTrimmedString(message, "airport");
        result->generatedAt = RequiredTrimmedString(message, "generated_at");
        result->authoritative = message.at("authoritative").get<bool>();

        for (const auto& item : message.at("values")) {
            if (!item.is_object()) throw std::invalid_argument("invalid AMAN gain/loss value");
            GainLossValue value;
            value.flightId = RequiredTrimmedString(item, "flight_id");
            value.callsign = NormalizeCallsign(RequiredTrimmedString(item, "callsign"));
            value.dataStatus = RequiredTrimmedString(item, "data_status");
            if (value.dataStatus != "fresh" && value.dataStatus != "stale" && value.dataStatus != "disconnected") {
                throw std::invalid_argument("invalid AMAN data status");
            }
            if (!item.contains("gain_loss_seconds") || !item.contains("reference_point") ||
                !item.contains("target_time") || !item.contains("predicted_time")) {
                throw std::invalid_argument("missing AMAN presentation value");
            }
            if (!item.at("gain_loss_seconds").is_null()) {
                if (!item.at("gain_loss_seconds").is_number_integer()) throw std::invalid_argument("invalid gain/loss value");
                value.seconds = item.at("gain_loss_seconds").get<long long>();
                value.referencePoint = RequiredTrimmedString(item, "reference_point");
                value.targetTime = RequiredTrimmedString(item, "target_time");
                value.predictedTime = RequiredTrimmedString(item, "predicted_time");
            } else if (!item.at("reference_point").is_null() || !item.at("target_time").is_null() ||
                       !item.at("predicted_time").is_null()) {
                throw std::invalid_argument("partial AMAN presentation value");
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
