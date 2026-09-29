#pragma once

#include <mutex>
#include <string>
#include <unordered_map>
#include <unordered_set>
#include <vector>

#include "generated/proto/euroscope.pb.h"

namespace FlightStrips::websocket {
    // Lives for the plugin process, across WebSocket generations. An acknowledged
    // command remains in seen_ so a late duplicate cannot execute again.
    class CommandOutbox {
    public:
        bool Begin(const std::string& id) {
            if (id.size() != 36) return false;
			for (size_t i = 0; i < id.size(); ++i) {
				if (i == 8 || i == 13 || i == 18 || i == 23) {
					if (id[i] != '-') return false;
				} else if (!((id[i] >= '0' && id[i] <= '9') || (id[i] >= 'a' && id[i] <= 'f'))) {
					return false;
				}
			}
            std::lock_guard lock(mutex_);
            return seen_.insert(id).second;
        }

        std::string Complete(const std::string& id,
                             flightstrips::euroscope::v1::CommandResultEvent::Status status,
                             flightstrips::euroscope::v1::CommandResultEvent::Reason reason,
                             std::string detail, int32_t sessionId = 0,
							 uint64_t ownerEpoch = 0, uint64_t masterEpoch = 0) {
            namespace wire = flightstrips::euroscope::v1;
            if (detail.size() > 256) {
				size_t end = 0;
				while (end < detail.size() && end < 256) {
					const auto first = static_cast<unsigned char>(detail[end]);
					const size_t width = first < 0x80 ? 1 : first < 0xe0 ? 2 : first < 0xf0 ? 3 : 4;
					if (end + width > 256) break;
					end += width;
				}
				detail.resize(end);
            }
            wire::Envelope envelope;
            envelope.set_command_id(id);
			envelope.set_session_id(sessionId);
			envelope.set_owner_epoch(ownerEpoch);
			envelope.set_master_epoch(masterEpoch);
            auto* result = envelope.mutable_command_result();
            result->set_command_id(id);
            result->set_status(status);
            result->set_reason(reason);
            result->set_detail(std::move(detail));
            std::string bytes;
            if (!envelope.SerializeToString(&bytes)) return {};
            std::lock_guard lock(mutex_);
            if (!seen_.contains(id)) return {};
            pending_[id] = bytes;
            return bytes;
        }

        void Acknowledge(const std::string& id) {
            std::lock_guard lock(mutex_);
            pending_.erase(id);
        }

        std::vector<std::string> Pending() const {
            std::lock_guard lock(mutex_);
            std::vector<std::string> result;
            result.reserve(pending_.size());
            for (const auto& [_, bytes] : pending_) result.push_back(bytes);
            return result;
        }

    private:
        mutable std::mutex mutex_;
        std::unordered_set<std::string> seen_;
        std::unordered_map<std::string, std::string> pending_;
    };
}
