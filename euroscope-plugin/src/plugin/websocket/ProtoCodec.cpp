#include "ProtoCodec.h"
#include "generated/proto/euroscope.pb.h"

#include <stdexcept>

namespace FlightStrips::websocket::protobuf {
    namespace {
        void CopyRunway(const Runway& source, wire::Runway* target) {
            target->set_name(source.name);
            target->set_departure(source.departure);
            target->set_arrival(source.arrival);
        }

        template <typename T>
        std::vector<flightplan::EcfmpRestriction> DecodeEcfmpRestrictions(const T& source) {
            std::vector<flightplan::EcfmpRestriction> result;
            result.reserve(source.size());
            for (const auto& restriction : source) {
                flightplan::EcfmpRestriction decoded;
                decoded.measure_id = restriction.measure_id();
                decoded.ident = restriction.ident();
                decoded.type = restriction.type();
                decoded.reason = restriction.reason();
                decoded.routes.assign(restriction.routes().begin(), restriction.routes().end());
                decoded.destination = restriction.destination();
                if (restriction.has_max_level()) decoded.max_level = restriction.max_level();
                if (restriction.has_min_level()) decoded.min_level = restriction.min_level();
                decoded.exact_levels.assign(restriction.exact_levels().begin(), restriction.exact_levels().end());
                decoded.has_ctot = restriction.has_ctot();
                result.push_back(std::move(decoded));
            }
            return result;
        }

        template <typename T>
        void CopyStrip(const T& source, wire::Strip* target) {
            target->set_callsign(source.callsign);
            target->set_origin(source.origin);
            target->set_destination(source.destination);
            target->set_alternate(source.alternate);
            target->set_route(source.route);
            target->set_remarks(source.remarks);
            target->set_runway(source.runway);
            target->set_squawk(source.squawk);
            target->set_assigned_squawk(source.assigned_squawk);
            target->set_sid(source.sid);
            target->set_star(source.star);
            target->set_cleared(source.cleared);
            target->set_ground_state(source.ground_state);
            target->set_cleared_altitude(source.cleared_altitude);
            target->set_requested_altitude(source.requested_altitude);
            target->set_heading(source.heading);
            target->set_aircraft_type(source.aircraft_type);
            target->set_aircraft_category(source.aircraft_category);
            target->set_spoken_callsign(source.spoken_callsign);
            auto* position = target->mutable_position();
            position->set_lat(source.position.lat);
            position->set_lon(source.position.lon);
            position->set_altitude(source.position.altitude);
            target->set_stand(source.stand);
            target->set_capabilities(source.capabilities);
            target->set_communication_type(source.communication_type);
            target->set_eobt(source.eobt);
            target->set_eldt(source.eldt);
            target->set_tracking_controller(source.tracking_controller);
            target->set_engine_type(source.engine_type);
            target->set_has_fp(source.has_fp);
            target->set_hold_supported(source.hold_supported);
            target->set_hold(source.hold);
            target->set_hold_type(source.hold_type);
            target->set_hold_eat(source.hold_eat);
        }

#define ENCODE_ONE(eventType, domainType, envelopeField, member) \
        case eventType: { \
            const auto& value = static_cast<const domainType&>(event); \
            envelope.mutable_##envelopeField()->set_##member(value.member); \
            break; \
        }

#define ENCODE_TWO(eventType, domainType, envelopeField, first, second) \
        case eventType: { \
            const auto& value = static_cast<const domainType&>(event); \
            auto* payload = envelope.mutable_##envelopeField(); \
            payload->set_##first(value.first); \
            payload->set_##second(value.second); \
            break; \
        }
    }

    std::string Serialize(const Event& event) {
        wire::Envelope envelope;
        switch (event.type) {
        case EVENT_TOKEN: {
            const auto& value = static_cast<const TokenEvent&>(event);
            auto* payload = envelope.mutable_token();
            payload->set_token(value.token);
            payload->set_version(value.version);
            break;
        }
        case EVENT_LOGIN: {
            const auto& value = static_cast<const LoginEvent&>(event);
            auto* payload = envelope.mutable_login();
            payload->set_airport(value.airport);
            payload->set_connection(value.connection);
            payload->set_position(value.position);
            payload->set_callsign(value.callsign);
            payload->set_range(value.range);
            payload->set_observer(value.observer);
            payload->set_local_ip(value.local_ip);
            break;
        }
        case EVENT_SYNC: {
            const auto& value = static_cast<const SyncEvent&>(event);
            auto* payload = envelope.mutable_sync();
            for (const auto& controller : value.controllers) {
                auto* target = payload->add_controllers();
                target->set_position(controller.position);
                target->set_callsign(controller.callsign);
            }
            for (const auto& strip : value.strips) CopyStrip(strip, payload->add_strips());
            for (const auto& runway : value.runways) CopyRunway(runway, payload->add_runways());
            for (const auto& sid : value.sids) {
                auto* target = payload->add_sids();
                target->set_name(sid.name);
                target->set_runway(sid.runway);
            }
            break;
        }
        case EVENT_STRIP_UPDATE: {
            const auto& value = static_cast<const StripUpdateEvent&>(event);
            CopyStrip(value, envelope.mutable_strip_update()->mutable_strip());
            break;
        }
        case EVENT_RUNWAY: {
            const auto& value = static_cast<const RunwayEvent&>(event);
            auto* payload = envelope.mutable_runway();
            for (const auto& runway : value.runways) CopyRunway(runway, payload->add_runways());
            break;
        }
        case EVENT_AIRCRAFT_POSITION_UPDATE: {
            const auto& value = static_cast<const PositionEvent&>(event);
            auto* payload = envelope.mutable_aircraft_position_update();
            payload->set_callsign(value.callsign);
            payload->set_lat(value.lat);
            payload->set_lon(value.lon);
            payload->set_altitude(value.altitude);
            break;
        }
        case EVENT_COORDINATION_RECEIVED: {
            const auto& value = static_cast<const CoordinationReceivedEvent&>(event);
            auto* payload = envelope.mutable_coordination_received();
            payload->set_callsign(value.callsign);
            payload->set_source_controller_callsign(value.source_controller_callsign);
            payload->set_controller_callsign(value.controller_callsign);
            break;
        }
        case EVENT_HOLD: {
            const auto& value = static_cast<const HoldEvent&>(event);
            auto* payload = envelope.mutable_hold();
            payload->set_callsign(value.callsign);
            payload->set_hold(value.hold);
            payload->set_hold_type(value.hold_type);
            payload->set_hold_eat(value.hold_eat);
            break;
        }
        case EVENT_PDC_STATE_CHANGE: {
            const auto& value = static_cast<const PdcStateChangeEvent&>(event);
            auto* payload = envelope.mutable_pdc_state_change();
            payload->set_callsign(value.callsign);
            payload->set_state(value.state);
            payload->set_pdc_request_remarks(value.pdc_request_remarks);
            break;
        }
        case EVENT_AMAN_ROUTE_FACT: {
            const auto& value = static_cast<const AMANRouteFactEvent&>(event);
            auto* payload = envelope.mutable_aman_route_fact();
            payload->set_version(value.version);
            auto* data = payload->mutable_data();
            data->set_callsign(value.data.callsign);
            data->set_kind(value.data.kind);
            if (value.data.direct_to_fix.has_value()) data->set_direct_to_fix(*value.data.direct_to_fix);
            data->set_observed_at(value.data.observed_at);
			if (value.data.assigned_speed.has_value()) {
				auto* assignedSpeed = data->mutable_assigned_speed();
				if (value.data.assigned_speed->knots.has_value()) assignedSpeed->set_knots(*value.data.assigned_speed->knots);
				if (value.data.assigned_speed->mach_thousandths.has_value()) assignedSpeed->set_mach_thousandths(*value.data.assigned_speed->mach_thousandths);
			}
            break;
        }
        ENCODE_TWO(EVENT_CONTROLLER_ONLINE, ControllerOnlineEvent, controller_online, position, callsign)
        ENCODE_ONE(EVENT_CONTROLLER_OFFLINE, ControllerOfflineEvent, controller_offline, callsign)
        ENCODE_TWO(EVENT_ASSIGNED_SQUAWK, AssignedSquawkEvent, assigned_squawk, callsign, squawk)
        ENCODE_TWO(EVENT_SQUAWK, SquawkEvent, squawk, callsign, squawk)
        ENCODE_TWO(EVENT_REQUESTED_ALTITUDE, RequestedAltitudeEvent, requested_altitude, callsign, altitude)
        ENCODE_TWO(EVENT_CLEARED_ALTITUDE, ClearedAltitudeEvent, cleared_altitude, callsign, altitude)
        ENCODE_TWO(EVENT_COMMUNICATION_TYPE, CommunicationTypeEvent, communication_type, callsign, communication_type)
        ENCODE_TWO(EVENT_GROUND_STATE, GroundStateEvent, ground_state, callsign, ground_state)
        ENCODE_TWO(EVENT_CLEARED_FLAG, ClearedFlagEvent, cleared_flag, callsign, cleared)
        ENCODE_TWO(EVENT_HEADING, HeadingEvent, heading, callsign, heading)
        ENCODE_ONE(EVENT_AIRCRAFT_DISCONNECT, AircraftDisconnectEvent, aircraft_disconnect, callsign)
        ENCODE_TWO(EVENT_STAND, StandEvent, stand, callsign, stand)
        ENCODE_TWO(EVENT_TRACKING_CONTROLLER_CHANGED, TrackingControllerChangedEvent, tracking_controller_changed, callsign, tracking_controller)
        ENCODE_TWO(EVENT_CDM_TOBT_UPDATE, CdmTobtUpdateEvent, cdm_tobt_update, callsign, tobt)
        ENCODE_TWO(EVENT_CDM_ASRT_TOGGLE, CdmAsrtToggleEvent, cdm_asrt_toggle, callsign, asrt)
        ENCODE_TWO(EVENT_CDM_TSAC_UPDATE, CdmTsacUpdateEvent, cdm_tsac_update, callsign, tsac)
        ENCODE_TWO(EVENT_CDM_DEICE_UPDATE, CdmDeiceUpdateEvent, cdm_deice_update, callsign, deice_type)
        ENCODE_TWO(EVENT_CDM_MANUAL_CTOT, CdmManualCtotEvent, cdm_manual_ctot, callsign, ctot)
        ENCODE_ONE(EVENT_CDM_CTOT_REMOVE, CdmCtotRemoveEvent, cdm_ctot_remove, callsign)
        ENCODE_ONE(EVENT_CDM_READY, CdmReadyEvent, cdm_ready, callsign)
        ENCODE_TWO(EVENT_ISSUE_PDC_CLEARANCE, IssuePdcClearanceEvent, issue_pdc_clearance, callsign, remarks)
        ENCODE_ONE(EVENT_PDC_REVERT_TO_VOICE, PdcRevertToVoiceEvent, pdc_revert_to_voice, callsign)
        ENCODE_TWO(EVENT_SEND_PRIVATE_MESSAGE, SendPrivateMessageEvent, send_private_message, callsign, message)
        default:
            throw std::runtime_error("unsupported protobuf event type");
        }

        std::string bytes;
        if (!envelope.SerializeToString(&bytes)) throw std::runtime_error("failed to serialize protobuf envelope");
        return bytes;
    }

    bool ParseEnvelope(const std::string& bytes, wire::Envelope& envelope) {
        return envelope.ParseFromString(bytes) && envelope.event_case() != wire::Envelope::EVENT_NOT_SET;
    }

    EventType GetEventType(const wire::Envelope& envelope) {
        return static_cast<EventType>(envelope.event_case());
    }

#define DECODE_ONE(wireType, domainType, member) \
    void Decode(const wire::wireType& source, domainType& target) { target.member = source.member(); }

#define DECODE_TWO(wireType, domainType, first, second) \
    void Decode(const wire::wireType& source, domainType& target) { \
        target.first = source.first(); \
        target.second = source.second(); \
    }

    DECODE_ONE(SessionInfoEvent, SessionInfoEvent, role)
    DECODE_TWO(AssignedSquawkEvent, AssignedSquawkEvent, callsign, squawk)
    DECODE_TWO(RequestedAltitudeEvent, RequestedAltitudeEvent, callsign, altitude)
    DECODE_TWO(ClearedAltitudeEvent, ClearedAltitudeEvent, callsign, altitude)
    DECODE_TWO(CommunicationTypeEvent, CommunicationTypeEvent, callsign, communication_type)
    DECODE_TWO(GroundStateEvent, GroundStateEvent, callsign, ground_state)
    DECODE_TWO(ClearedFlagEvent, ClearedFlagEvent, callsign, cleared)
    DECODE_TWO(HeadingEvent, HeadingEvent, callsign, heading)
    DECODE_TWO(StandEvent, StandEvent, callsign, stand)
    DECODE_TWO(EobtEvent, EobtEvent, callsign, eobt)
    DECODE_ONE(GenerateSquawkEvent, GenerateSquawkEvent, callsign)
    DECODE_TWO(RouteEvent, RouteEvent, callsign, route)
    DECODE_TWO(RemarksEvent, RemarksEvent, callsign, remarks)
    DECODE_TWO(AircraftInfoEvent, AircraftInfoEvent, callsign, aircraft_type)
    DECODE_TWO(SidEvent, SidEvent, callsign, sid)
    DECODE_TWO(AircraftRunwayEvent, AircraftRunwayEvent, callsign, runway)
    DECODE_TWO(CoordinationHandoverEvent, CoordinationHandoverEvent, callsign, target_callsign)
    DECODE_ONE(AssumeOnlyEvent, AssumeOnlyEvent, callsign)
    DECODE_ONE(AssumeAndDropEvent, AssumeAndDropEvent, callsign)
    DECODE_ONE(DropTrackingEvent, DropTrackingEvent, callsign)
    DECODE_TWO(SendPrivateMessageEvent, SendPrivateMessageEvent, callsign, message)

    void Decode(const wire::RunwayMismatchAlertEvent& source, RunwayMismatchAlertEvent& target) {
        target.expected_departure.assign(source.expected_departure().begin(), source.expected_departure().end());
        target.expected_arrival.assign(source.expected_arrival().begin(), source.expected_arrival().end());
        target.current_departure.assign(source.current_departure().begin(), source.current_departure().end());
        target.current_arrival.assign(source.current_arrival().begin(), source.current_arrival().end());
    }

    void Decode(const wire::CdmUpdateEvent& source, CdmUpdateEvent& target) {
        target.callsign = source.callsign();
        target.eobt = source.eobt();
        target.tobt = source.tobt();
        target.tobt_confirmed_by = source.tobt_confirmed_by();
        target.tsat = source.tsat();
        target.ttot = source.ttot();
        target.ctot = source.ctot();
        target.asrt = source.asrt();
        target.tsac = source.tsac();
        target.asat = source.asat();
        target.status = source.status();
        target.manual_ctot = source.manual_ctot();
        target.deice_type = source.deice_type();
        target.ecfmp_id = source.ecfmp_id();
        target.phase = source.phase();
        target.ecfmp_restrictions = DecodeEcfmpRestrictions(source.ecfmp_restrictions());
    }

    void Decode(const wire::CdmUpdateBatchEvent& source, CdmUpdateBatchEvent& target) {
        target.updates.clear();
        target.updates.reserve(source.updates_size());
        for (const auto& update : source.updates()) {
            CdmUpdateEvent decoded;
            Decode(update, decoded);
            target.updates.push_back(std::move(decoded));
        }
    }

    void Decode(const wire::AircraftInfoRemarksEvent& source, AircraftInfoRemarksEvent& target) {
        target.callsign = source.callsign();
        target.aircraft_type = source.aircraft_type();
        target.remarks = source.remarks();
    }

    void Decode(const wire::BackendSyncEvent& source, BackendSyncEvent& target) {
        target.latitude = source.latitude();
        target.longitude = source.longitude();
        target.strips.clear();
        target.strips.reserve(source.strips_size());
        for (const auto& strip : source.strips()) {
            BackendSyncStrip decoded;
            decoded.callsign = strip.callsign();
            decoded.assigned_squawk = strip.assigned_squawk();
            decoded.cleared = strip.cleared();
            decoded.ground_state = strip.ground_state();
            decoded.stand = strip.stand();
            decoded.pdc_state = strip.pdc_state();
            decoded.pdc_request_remarks = strip.pdc_request_remarks();
            if (strip.has_cdm()) {
                const auto& cdm = strip.cdm();
                decoded.cdm.eobt = cdm.eobt();
                decoded.cdm.tobt = cdm.tobt();
                decoded.cdm.tobt_confirmed_by = cdm.tobt_confirmed_by();
                decoded.cdm.tsat = cdm.tsat();
                decoded.cdm.ttot = cdm.ttot();
                decoded.cdm.ctot = cdm.ctot();
                decoded.cdm.asrt = cdm.asrt();
                decoded.cdm.tsac = cdm.tsac();
                decoded.cdm.asat = cdm.asat();
                decoded.cdm.status = cdm.status();
                decoded.cdm.manual_ctot = cdm.manual_ctot();
                decoded.cdm.deice_type = cdm.deice_type();
                decoded.cdm.ecfmp_id = cdm.ecfmp_id();
                decoded.cdm.phase = cdm.phase();
                decoded.cdm.ecfmp_restrictions = DecodeEcfmpRestrictions(cdm.ecfmp_restrictions());
            }
            target.strips.push_back(std::move(decoded));
        }
    }

    void Decode(const wire::CreateFPLEvent& source, CreateFPLEvent& target) {
        target.callsign = source.callsign();
        target.origin = source.origin();
        target.destination = source.destination();
        target.alternate_ad = source.alternate_ad();
        target.sid = source.sid();
        target.assigned_squawk = source.assigned_squawk();
        target.eobt = source.eobt();
        target.aircraft_type = source.aircraft_type();
        target.requested_altitude = source.requested_altitude();
        target.route = source.route();
        target.stand = source.stand();
        target.runway = source.runway();
        target.remarks = source.remarks();
        target.persons_on_board = source.persons_on_board();
        target.fpl_type = source.fpl_type();
        target.language = source.language();
    }

    void Decode(const wire::PdcStateChangeEvent& source, PdcStateChangeEvent& target) {
        target.callsign = source.callsign();
        target.state = source.state();
        target.pdc_request_remarks = source.pdc_request_remarks();
    }

#undef ENCODE_ONE
#undef ENCODE_TWO
#undef DECODE_ONE
#undef DECODE_TWO
}
