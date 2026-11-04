#pragma once
#include <cmath>
#include <string>
#include <unordered_map>
#include <unordered_set>
#include "generated/proto/euroscope.pb.h"

namespace FlightStrips::websocket {
    enum ClientState { STATE_UNKNOWN, STATE_SLAVE, STATE_MASTER, STATE_OBSERVER };
    namespace membership_wire = ::flightstrips::euroscope::v1;

    struct AirportMembership {
        membership_wire::MembershipContext context;
        ClientState role = STATE_UNKNOWN;
        std::string masterCallsign;
        std::unordered_set<std::string> inventory;
        std::unordered_set<std::string> excluded;
        double latitude = 0, longitude = 0;
        bool hasCoordinates = false;

        bool Includes(const membership_wire::Strip& strip) const {
            if (strip.origin() == context.airport() || strip.destination() == context.airport()) return true;
            if (strip.has_fp() && !strip.origin().empty() && strip.origin() != "ZZZZ" &&
                !strip.destination().empty() && strip.destination() != "ZZZZ") return false;
            if (!hasCoordinates || !strip.has_position()) return false;
            constexpr double radians = 3.14159265358979323846 / 180;
            const double dlat=(strip.position().lat()-latitude)*radians;
            const double dlon=(strip.position().lon()-longitude)*radians;
            const double a=std::sin(dlat/2)*std::sin(dlat/2)+std::cos(latitude*radians)*
                std::cos(strip.position().lat()*radians)*std::sin(dlon/2)*std::sin(dlon/2);
            return 3440.065 * 2 * std::asin(std::sqrt(std::min(1.0,a))) <= 30;
        }
    };

    // Accessed on the EuroScope callback thread. The websocket thread only queues
    // immutable bytes; acceptance happens immediately before SDK application.
    class MembershipState {
    public:
        std::unordered_map<std::string, AirportMembership> members;
        std::unordered_set<std::string> retired;
        void Reset() { members.clear(); retired.clear(); }

        static bool IsSdkCommand(const membership_wire::Envelope& event) {
            using E=membership_wire::Envelope;
            switch(event.event_case()) {
            case E::kAssignedSquawk:case E::kRequestedAltitude:case E::kClearedAltitude:case E::kCommunicationType:
            case E::kGroundState:case E::kClearedFlag:case E::kHeading:case E::kStand:case E::kEobt:case E::kGenerateSquawk:
            case E::kRoute:case E::kRemarks:case E::kAircraftInfo:case E::kAircraftInfoRemarks:case E::kSid:case E::kAircraftRunway:
            case E::kCoordinationHandover:case E::kAssumeOnly:case E::kAssumeAndDrop:case E::kDropTracking:case E::kCreateFpl:
            case E::kSendPrivateMessage:case E::kSendFrequencyMessage:case E::kHold:return true;
            default:return false;
            }
        }

        bool Accept(const membership_wire::Envelope& event, const std::string& instance,
                    uint64_t transport, const std::string& environment) {
            if (!event.has_context() || transport==0) return false;
            const auto& c=event.context();
            if (c.instance_id()!=instance || c.transport_generation()!=transport || c.membership_id().empty() ||
                c.generation()==0 || c.session_id()<=0 || c.airport().empty() || c.environment()!=environment || retired.contains(c.membership_id())) return false;
            const bool humanCommand=event.command_origin()==membership_wire::COMMAND_ORIGIN_HUMAN && IsSdkCommand(event);
            auto found=members.find(c.membership_id());
            if(found==members.end() && IsSdkCommand(event)) return false;
            if (found!=members.end()) {
                const auto& old=found->second.context;
                if (old.generation()!=c.generation() || old.session_id()!=c.session_id() || old.airport()!=c.airport() ||
                    old.environment()!=c.environment()) return false;
                if (!event.has_membership_removed() && !humanCommand && c.master_epoch()<old.master_epoch()) return false;
                if (!event.has_membership_removed() && !humanCommand && c.master_epoch()>old.master_epoch() && !event.has_session_info() && !event.has_master_info()) return false;
            }
            if (event.has_membership_removed()) {
                retired.insert(c.membership_id()); members.erase(c.membership_id()); return false;
            }
            auto& member=members[c.membership_id()];
            const auto currentEpoch=member.context.master_epoch();
            member.context=c;
            if(humanCommand) member.context.set_master_epoch(currentEpoch);
            if (event.has_session_info()) {
                const auto& role=event.session_info().role();
                member.role=role=="master"?STATE_MASTER:role=="observer"?STATE_OBSERVER:role=="slave"?STATE_SLAVE:STATE_UNKNOWN;
            }
            if (event.has_master_info()) member.masterCallsign=event.master_info().master_callsign();
            if (event.has_backend_sync()) {
                member.latitude=event.backend_sync().latitude(); member.longitude=event.backend_sync().longitude();member.hasCoordinates=true;
                for(const auto& strip:event.backend_sync().strips()) member.inventory.insert(strip.callsign());
            }
            return true;
        }
    };
}
