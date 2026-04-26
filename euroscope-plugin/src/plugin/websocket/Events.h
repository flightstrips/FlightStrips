#ifndef EVENTS_H
#define EVENTS_H
#include <nlohmann/json.hpp>
#include <utility>

#define EVENT_UNKNOWN_NAME "unknown"
#define EVENT_TOKEN_NAME "token"
#define EVENT_LOGIN_NAME "login"
#define EVENT_CONTROLLER_ONLINE_NAME "controller_online"
#define EVENT_CONTROLLER_OFFLINE_NAME "controller_offline"
#define EVENT_SYNC_NAME "sync"
#define EVENT_ASSIGNED_SQUAWK_NAME "assigned_squawk"
#define EVENT_SQUAWK_NAME "squawk"
#define EVENT_REQUESTED_ALTITUDE_NAME "requested_altitude"
#define EVENT_CLEARED_ALTITUDE_NAME "cleared_altitude"
#define EVENT_COMMUNICATION_TYPE_NAME "communication_type"
#define EVENT_GROUND_STATE_NAME "ground_state"
#define EVENT_CLEARED_FLAG_NAME "cleared_flag"
#define EVENT_AIRCRAFT_POSITION_UPDATE_NAME "aircraft_position_update"
#define EVENT_HEADING_NAME "heading"
#define EVENT_AIRCRAFT_DISCONNECT_NAME "aircraft_disconnect"
#define EVENT_STAND_NAME "stand"
#define EVENT_TRACKING_CONTROLLER_CHANGED_NAME "tracking_controller_changed"
#define EVENT_STRIP_UPDATE_NAME "strip_update"
#define EVENT_RUNWAY_NAME "runway"
#define EVENT_SESSION_INFO_NAME "session_info"
#define EVENT_RUNWAY_MISMATCH_ALERT_NAME "runway_mismatch_alert"
#define EVENT_GENERATE_SQUAWK_NAME "generate_squawk"
#define EVENT_ROUTE_NAME "route"
#define EVENT_REMARKS_NAME "remarks"
#define EVENT_SID_NAME "sid"
#define EVENT_AIRCRAFT_RUNWAY_NAME "aircraft_runway"
#define EVENT_COORDINATION_HANDOVER_NAME "coordination_handover"
#define EVENT_COORDINATION_RECEIVED_NAME "coordination_received"
#define EVENT_ASSUME_ONLY_NAME "assume_only"
#define EVENT_ASSUME_AND_DROP_NAME "assume_and_drop"
#define EVENT_DROP_TRACKING_NAME "drop_tracking"
#define EVENT_BACKEND_SYNC_NAME "backend_sync"
#define EVENT_CREATE_FPL_NAME "create_fpl"
#define EVENT_CDM_UPDATE_NAME "cdm_update"
#define EVENT_CDM_TOBT_UPDATE_NAME "cdm_tobt_update"
#define EVENT_CDM_ASRT_TOGGLE_NAME "cdm_asrt_toggle"
#define EVENT_CDM_TSAC_UPDATE_NAME "cdm_tsac_update"
#define EVENT_CDM_DEICE_UPDATE_NAME "cdm_deice_update"
#define EVENT_CDM_MANUAL_CTOT_NAME "cdm_manual_ctot"
#define EVENT_CDM_CTOT_REMOVE_NAME "cdm_ctot_remove"
#define EVENT_CDM_APPROVE_REQ_TOBT_NAME "cdm_approve_req_tobt"
#define EVENT_CDM_MASTER_TOGGLE_NAME "cdm_master_toggle"
#define EVENT_PDC_STATE_CHANGE_NAME "pdc_state_change"
#define EVENT_ISSUE_PDC_CLEARANCE_NAME "issue_pdc_clearance"
#define EVENT_PDC_REVERT_TO_VOICE_NAME "pdc_revert_to_voice"

enum EventType {
    EVENT_UNKNOWN = 0,
    EVENT_TOKEN,
    EVENT_LOGIN,
    EVENT_CONTROLLER_ONLINE,
    EVENT_CONTROLLER_OFFLINE,
    EVENT_SYNC,
    EVENT_ASSIGNED_SQUAWK,
    EVENT_SQUAWK,
    EVENT_REQUESTED_ALTITUDE,
    EVENT_CLEARED_ALTITUDE,
    EVENT_COMMUNICATION_TYPE,
    EVENT_GROUND_STATE,
    EVENT_CLEARED_FLAG,
    EVENT_AIRCRAFT_POSITION_UPDATE,
    EVENT_HEADING,
    EVENT_AIRCRAFT_DISCONNECT,
    EVENT_STAND,
    EVENT_TRACKING_CONTROLLER_CHANGED,
    EVENT_STRIP_UPDATE,
    EVENT_RUNWAY,
    // Server only events:
    EVENT_SESSION_INFO,
    EVENT_RUNWAY_MISMATCH_ALERT,
    EVENT_GENERATE_SQUAWK,
    EVENT_ROUTE,
    EVENT_REMARKS,
    EVENT_SID,
    EVENT_AIRCRAFT_RUNWAY,
    EVENT_COORDINATION_HANDOVER,
    EVENT_COORDINATION_RECEIVED,
    EVENT_ASSUME_ONLY,
    EVENT_ASSUME_AND_DROP,
    EVENT_DROP_TRACKING,
    EVENT_BACKEND_SYNC,
    EVENT_CREATE_FPL,
    EVENT_CDM_UPDATE,
    EVENT_CDM_TOBT_UPDATE,
    EVENT_CDM_ASRT_TOGGLE,
    EVENT_CDM_TSAC_UPDATE,
    EVENT_CDM_DEICE_UPDATE,
    EVENT_CDM_MANUAL_CTOT,
    EVENT_CDM_CTOT_REMOVE,
    EVENT_CDM_APPROVE_REQ_TOBT,
    EVENT_CDM_MASTER_TOGGLE,
    EVENT_PDC_STATE_CHANGE,
    EVENT_ISSUE_PDC_CLEARANCE,
    EVENT_PDC_REVERT_TO_VOICE,
};

NLOHMANN_JSON_SERIALIZE_ENUM(EventType, {
                             {EVENT_UNKNOWN, EVENT_UNKNOWN_NAME},
                             {EVENT_TOKEN, EVENT_TOKEN_NAME},
                             {EVENT_LOGIN, EVENT_LOGIN_NAME},
                             {EVENT_CONTROLLER_ONLINE, EVENT_CONTROLLER_ONLINE_NAME},
                             {EVENT_CONTROLLER_OFFLINE, EVENT_CONTROLLER_OFFLINE_NAME},
                             {EVENT_SYNC, EVENT_SYNC_NAME},
                             {EVENT_ASSIGNED_SQUAWK, EVENT_ASSIGNED_SQUAWK_NAME},
                             {EVENT_SQUAWK, EVENT_SQUAWK_NAME},
                             {EVENT_REQUESTED_ALTITUDE, EVENT_REQUESTED_ALTITUDE_NAME},
                             {EVENT_CLEARED_ALTITUDE, EVENT_CLEARED_ALTITUDE_NAME},
                             {EVENT_COMMUNICATION_TYPE, EVENT_COMMUNICATION_TYPE_NAME},
                             {EVENT_GROUND_STATE, EVENT_GROUND_STATE_NAME},
                             {EVENT_CLEARED_FLAG, EVENT_CLEARED_FLAG_NAME},
                             {EVENT_AIRCRAFT_POSITION_UPDATE, EVENT_AIRCRAFT_POSITION_UPDATE_NAME},
                              {EVENT_HEADING, EVENT_HEADING_NAME},
                              {EVENT_AIRCRAFT_DISCONNECT, EVENT_AIRCRAFT_DISCONNECT_NAME},
                              {EVENT_STAND, EVENT_STAND_NAME},
                              {EVENT_TRACKING_CONTROLLER_CHANGED, EVENT_TRACKING_CONTROLLER_CHANGED_NAME},
                               {EVENT_STRIP_UPDATE, EVENT_STRIP_UPDATE_NAME},
                               {EVENT_RUNWAY, EVENT_RUNWAY_NAME},
                               {EVENT_SESSION_INFO, EVENT_SESSION_INFO_NAME},
                               {EVENT_RUNWAY_MISMATCH_ALERT, EVENT_RUNWAY_MISMATCH_ALERT_NAME},
                               {EVENT_GENERATE_SQUAWK, EVENT_GENERATE_SQUAWK_NAME},
                              {EVENT_ROUTE, EVENT_ROUTE_NAME},
                              {EVENT_REMARKS, EVENT_REMARKS_NAME},
                              {EVENT_SID, EVENT_SID_NAME},
                               {EVENT_AIRCRAFT_RUNWAY, EVENT_AIRCRAFT_RUNWAY_NAME},
                               {EVENT_COORDINATION_HANDOVER, EVENT_COORDINATION_HANDOVER_NAME},
                               {EVENT_COORDINATION_RECEIVED, EVENT_COORDINATION_RECEIVED_NAME},
                               {EVENT_ASSUME_ONLY, EVENT_ASSUME_ONLY_NAME},
                                {EVENT_ASSUME_AND_DROP, EVENT_ASSUME_AND_DROP_NAME},
                                {EVENT_DROP_TRACKING, EVENT_DROP_TRACKING_NAME},
                                 {EVENT_BACKEND_SYNC, EVENT_BACKEND_SYNC_NAME},
                                  {EVENT_CREATE_FPL, EVENT_CREATE_FPL_NAME},
                                   {EVENT_CDM_UPDATE, EVENT_CDM_UPDATE_NAME},
                                  {EVENT_CDM_TOBT_UPDATE, EVENT_CDM_TOBT_UPDATE_NAME},
                                  {EVENT_CDM_ASRT_TOGGLE, EVENT_CDM_ASRT_TOGGLE_NAME},
                                  {EVENT_CDM_TSAC_UPDATE, EVENT_CDM_TSAC_UPDATE_NAME},
                                  {EVENT_CDM_DEICE_UPDATE, EVENT_CDM_DEICE_UPDATE_NAME},
                                  {EVENT_CDM_MANUAL_CTOT, EVENT_CDM_MANUAL_CTOT_NAME},
                                  {EVENT_CDM_CTOT_REMOVE, EVENT_CDM_CTOT_REMOVE_NAME},
                                 {EVENT_CDM_APPROVE_REQ_TOBT, EVENT_CDM_APPROVE_REQ_TOBT_NAME},
                                 {EVENT_CDM_MASTER_TOGGLE, EVENT_CDM_MASTER_TOGGLE_NAME},
                                 {EVENT_PDC_STATE_CHANGE, EVENT_PDC_STATE_CHANGE_NAME},
                                 {EVENT_ISSUE_PDC_CLEARANCE, EVENT_ISSUE_PDC_CLEARANCE_NAME},
                                 {EVENT_PDC_REVERT_TO_VOICE, EVENT_PDC_REVERT_TO_VOICE_NAME},
                                 })

struct Event {
    EventType type{EVENT_UNKNOWN};

protected:
    explicit Event(const EventType type) : type(type) {
    }

    Event() = default;
};

struct TokenEvent final : Event {
    std::string token;

    explicit TokenEvent(std::string token) : Event(EVENT_TOKEN), token(std::move(token)) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(TokenEvent, token, type)
};

struct LoginEvent final : Event {
    std::string airport;
    std::string connection;
    std::string position;
    std::string callsign;
    int range;
    bool observer{false};

    LoginEvent(std::string airport, std::string connection, std::string position, std::string callsign, const int range, const bool observer)
        : Event(EVENT_LOGIN),
          airport(std::move(airport)),
          connection(std::move(connection)),
          position(std::move(position)),
          callsign(std::move(callsign)),
          range(range),
          observer(observer) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(LoginEvent, airport, connection, position, callsign, range, observer, type);
};

struct Runway final {
    std::string name;
    bool departure;
    bool arrival;
    NLOHMANN_DEFINE_TYPE_INTRUSIVE(Runway, name, departure, arrival);
};

struct SidEntry final {
    std::string name;
    std::string runway;
    NLOHMANN_DEFINE_TYPE_INTRUSIVE(SidEntry, name, runway);
};

struct RunwayEvent final : Event {
    std::vector<Runway> runways;

    explicit RunwayEvent(std::vector<Runway> runways) : Event(EVENT_RUNWAY), runways(std::move(runways)) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(RunwayEvent, runways, type);
};

struct CdmUpdateEvent final : Event {
    std::string callsign;
    std::string eobt;
    std::string tobt;
    std::string req_tobt;
    std::string req_tobt_source;
    std::string tobt_confirmed_by;
    std::string tsat;
    std::string ttot;
    std::string ctot;
    std::string asrt;
    std::string tsac;
    std::string asat;
    std::string status;
    std::string manual_ctot;
    std::string deice_type;
    std::string ecfmp_id;
    std::string phase;

    CdmUpdateEvent()
        : Event(EVENT_CDM_UPDATE) {
    }

    CdmUpdateEvent(
        std::string callsign,
        std::string eobt,
        std::string tobt,
        std::string req_tobt,
        std::string req_tobt_source,
        std::string tsat,
        std::string ttot,
        std::string ctot,
        std::string asrt,
        std::string tsac,
        std::string asat,
        std::string status,
        std::string manual_ctot,
        std::string deice_type,
        std::string ecfmp_id,
        std::string phase = {},
        std::string tobt_confirmed_by = {}
    ) : Event(EVENT_CDM_UPDATE),
        callsign(std::move(callsign)),
        eobt(std::move(eobt)),
        tobt(std::move(tobt)),
        req_tobt(std::move(req_tobt)),
        req_tobt_source(std::move(req_tobt_source)),
        tobt_confirmed_by(std::move(tobt_confirmed_by)),
        tsat(std::move(tsat)),
        ttot(std::move(ttot)),
        ctot(std::move(ctot)),
        asrt(std::move(asrt)),
        tsac(std::move(tsac)),
        asat(std::move(asat)),
        status(std::move(status)),
        manual_ctot(std::move(manual_ctot)),
        deice_type(std::move(deice_type)),
        ecfmp_id(std::move(ecfmp_id)),
        phase(std::move(phase)) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE_WITH_DEFAULT(
        CdmUpdateEvent,
        callsign,
        eobt,
        tobt,
        req_tobt,
        req_tobt_source,
        tobt_confirmed_by,
        tsat,
        ttot,
        ctot,
        asrt,
        tsac,
        asat,
        status,
        manual_ctot,
        deice_type,
        ecfmp_id,
        phase,
        type
    );
};

struct CdmTobtUpdateEvent final : Event {
    std::string callsign;
    std::string tobt;

    CdmTobtUpdateEvent() : Event(EVENT_CDM_TOBT_UPDATE) {}
    CdmTobtUpdateEvent(std::string callsign, std::string tobt)
        : Event(EVENT_CDM_TOBT_UPDATE), callsign(std::move(callsign)), tobt(std::move(tobt)) {}

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(CdmTobtUpdateEvent, callsign, tobt, type);
};

struct CdmAsrtToggleEvent final : Event {
    std::string callsign;
    std::string asrt;

    CdmAsrtToggleEvent() : Event(EVENT_CDM_ASRT_TOGGLE) {}
    CdmAsrtToggleEvent(std::string callsign, std::string asrt)
        : Event(EVENT_CDM_ASRT_TOGGLE), callsign(std::move(callsign)), asrt(std::move(asrt)) {}

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(CdmAsrtToggleEvent, callsign, asrt, type);
};

struct CdmTsacUpdateEvent final : Event {
    std::string callsign;
    std::string tsac;

    CdmTsacUpdateEvent() : Event(EVENT_CDM_TSAC_UPDATE) {}
    CdmTsacUpdateEvent(std::string callsign, std::string tsac)
        : Event(EVENT_CDM_TSAC_UPDATE), callsign(std::move(callsign)), tsac(std::move(tsac)) {}

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(CdmTsacUpdateEvent, callsign, tsac, type);
};

struct CdmDeiceUpdateEvent final : Event {
    std::string callsign;
    std::string deice_type;

    CdmDeiceUpdateEvent() : Event(EVENT_CDM_DEICE_UPDATE) {}
    CdmDeiceUpdateEvent(std::string callsign, std::string deiceType)
        : Event(EVENT_CDM_DEICE_UPDATE), callsign(std::move(callsign)), deice_type(std::move(deiceType)) {}

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(CdmDeiceUpdateEvent, callsign, deice_type, type);
};

struct CdmManualCtotEvent final : Event {
    std::string callsign;
    std::string ctot;

    CdmManualCtotEvent() : Event(EVENT_CDM_MANUAL_CTOT) {}
    CdmManualCtotEvent(std::string callsign, std::string ctot)
        : Event(EVENT_CDM_MANUAL_CTOT), callsign(std::move(callsign)), ctot(std::move(ctot)) {}

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(CdmManualCtotEvent, callsign, ctot, type);
};

struct CdmCtotRemoveEvent final : Event {
    std::string callsign;

    CdmCtotRemoveEvent() : Event(EVENT_CDM_CTOT_REMOVE) {}
    explicit CdmCtotRemoveEvent(std::string callsign)
        : Event(EVENT_CDM_CTOT_REMOVE), callsign(std::move(callsign)) {}

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(CdmCtotRemoveEvent, callsign, type);
};

struct CdmApproveReqTobtEvent final : Event {
    std::string callsign;

    CdmApproveReqTobtEvent() : Event(EVENT_CDM_APPROVE_REQ_TOBT) {}
    explicit CdmApproveReqTobtEvent(std::string callsign)
        : Event(EVENT_CDM_APPROVE_REQ_TOBT), callsign(std::move(callsign)) {}

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(CdmApproveReqTobtEvent, callsign, type);
};

struct CdmMasterToggleEvent final : Event {
    bool master{false};

    CdmMasterToggleEvent() : Event(EVENT_CDM_MASTER_TOGGLE) {}
    explicit CdmMasterToggleEvent(const bool master)
        : Event(EVENT_CDM_MASTER_TOGGLE), master(master) {}

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(CdmMasterToggleEvent, master, type);
};

struct BackendSyncCdmData final {
    std::string eobt;
    std::string tobt;
    std::string req_tobt;
    std::string req_tobt_source;
    std::string tobt_confirmed_by;
    std::string tsat;
    std::string ttot;
    std::string ctot;
    std::string asrt;
    std::string tsac;
    std::string asat;
    std::string status;
    std::string manual_ctot;
    std::string deice_type;
    std::string ecfmp_id;
    std::string phase;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE_WITH_DEFAULT(
        BackendSyncCdmData,
        eobt,
        tobt,
        req_tobt,
        req_tobt_source,
        tobt_confirmed_by,
        tsat,
        ttot,
        ctot,
        asrt,
        tsac,
        asat,
        status,
        manual_ctot,
        deice_type,
        ecfmp_id,
        phase
    );
};

struct AssignedSquawkEvent final : Event {
    std::string callsign;
    std::string squawk;

    AssignedSquawkEvent(std::string callsign, std::string squawk) : Event(EVENT_ASSIGNED_SQUAWK),
                                                                    callsign(std::move(callsign)),
                                                                    squawk(std::move(squawk)) {
    }
    AssignedSquawkEvent() = default;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(AssignedSquawkEvent, callsign, squawk, type);
};

struct SquawkEvent final : Event {
    std::string callsign;
    std::string squawk;

    SquawkEvent(std::string callsign, std::string squawk) : Event(EVENT_SQUAWK), callsign(std::move(callsign)),
                                                            squawk(std::move(squawk)) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(SquawkEvent, callsign, squawk, type);
};

struct HeadingEvent final : Event {
    std::string callsign;
    int heading;

    HeadingEvent(std::string callsign, const int heading) : Event(EVENT_HEADING), callsign(std::move(callsign)),
                                                            heading(heading) {
    }
    HeadingEvent() = default;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(HeadingEvent, callsign, heading, type);
};

struct RequestedAltitudeEvent final : Event {
    std::string callsign;
    int altitude;

    explicit RequestedAltitudeEvent(std::string callsign, const int altitude) : Event(EVENT_REQUESTED_ALTITUDE),
        callsign(std::move(callsign)), altitude(altitude) {
    }
    RequestedAltitudeEvent() = default;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(RequestedAltitudeEvent, callsign, altitude, type);
};

struct ClearedAltitudeEvent final : Event {
    std::string callsign;
    int altitude;

    explicit ClearedAltitudeEvent(std::string callsign, const int altitude) : Event(EVENT_CLEARED_ALTITUDE),
                                                                              callsign(std::move(callsign)),
                                                                              altitude(altitude) {
    }
    ClearedAltitudeEvent() = default;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(ClearedAltitudeEvent, callsign, altitude, type);
};

struct CommunicationTypeEvent final : Event {
    std::string callsign;
    std::string communication_type;

    explicit
    CommunicationTypeEvent(std::string callsign, const char communicationType) : Event(EVENT_COMMUNICATION_TYPE),
        callsign(std::move(callsign)), communication_type({communicationType}) {
    }
    CommunicationTypeEvent() = default;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(CommunicationTypeEvent, callsign, communication_type, type);
};

struct GroundStateEvent final : Event {
    std::string callsign;
    std::string ground_state;

    explicit GroundStateEvent(std::string callsign, std::string groundState) : Event(EVENT_GROUND_STATE),
                                                                               callsign(std::move(callsign)),
                                                                               ground_state(std::move(groundState)) {
    }
    GroundStateEvent() = default;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(GroundStateEvent, callsign, ground_state, type);
};

struct ClearedFlagEvent final : Event {
    std::string callsign;
    bool cleared;

    explicit ClearedFlagEvent(std::string callsign, const bool cleared) : Event(EVENT_CLEARED_FLAG),
                                                                          callsign(std::move(callsign)),
                                                                          cleared(cleared) {
    }
    ClearedFlagEvent() = default;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(ClearedFlagEvent, callsign, cleared, type);
};

struct PositionEvent final : Event {
    std::string callsign;
    double lat;
    double lon;
    int altitude;

    explicit PositionEvent(std::string callsign, const double lat, const double lon,
                           const int altitude) : Event(EVENT_AIRCRAFT_POSITION_UPDATE),
                                                 callsign(std::move(callsign)),
                                                 lat(lat), lon(lon), altitude(altitude) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(PositionEvent, callsign, lat, lon, altitude, type);
};

struct AircraftDisconnectEvent final : Event {
    std::string callsign;

    explicit AircraftDisconnectEvent(std::string callsign) : Event(EVENT_AIRCRAFT_DISCONNECT),
                                                             callsign(std::move(callsign)) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(AircraftDisconnectEvent, callsign, type);
};

struct Position final {
    double lat;
    double lon;
    int altitude;

    explicit Position(const double lat, const double lon, const int altitude) : lat(lat), lon(lon), altitude(altitude) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(Position, lat, lon, altitude);
};

struct StripUpdateEvent final : Event {
    StripUpdateEvent(std::string callsign, std::string origin, std::string destination, std::string alternate, std::string route,
          std::string remarks, std::string runway, std::string squawk, std::string assigned_squawk, std::string sid,
          bool cleared, std::string ground_state, int cleared_altitude, int requested_altitude, int heading,
          std::string aircraft_type, std::string aircraft_category, Position position, std::string stand,
          std::string communication_type, std::string capabilities, std::string eobt, std::string eldt,
          std::string tracking_controller, std::string engine_type, bool has_fp = true)
        : Event(EVENT_STRIP_UPDATE), callsign(std::move(callsign)),
          origin(std::move(origin)),
          destination(std::move(destination)),
          alternate(std::move(alternate)),
          route(std::move(route)),
          remarks(std::move(remarks)),
          runway(std::move(runway)),
          squawk(std::move(squawk)),
          assigned_squawk(std::move(assigned_squawk)),
          sid(std::move(sid)),
          cleared(cleared),
          ground_state(std::move(ground_state)),
          cleared_altitude(cleared_altitude),
          requested_altitude(requested_altitude),
          heading(heading),
          aircraft_type(std::move(aircraft_type)),
          aircraft_category(std::move(aircraft_category)),
          position(position),
          stand(std::move(stand)),
          communication_type(std::move(communication_type)),
          capabilities(std::move(capabilities)),
          eobt(std::move(eobt)),
          eldt(std::move(eldt)),
          tracking_controller(std::move(tracking_controller)),
          engine_type(std::move(engine_type)),
          has_fp(has_fp) {
    }

    std::string callsign;
    std::string origin;
    std::string destination;
    std::string alternate;
    std::string route;
    std::string remarks;
    std::string runway;
    std::string squawk;
    std::string assigned_squawk;
    std::string sid;
    bool cleared;
    std::string ground_state;
    int cleared_altitude;
    int requested_altitude;
    int heading;
    std::string aircraft_type;
    std::string aircraft_category;
    Position position;
    std::string stand;
    std::string communication_type;
    std::string capabilities;
    std::string eobt;
    std::string eldt;
    std::string tracking_controller;
    std::string engine_type;
    bool has_fp;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(StripUpdateEvent, callsign, origin, destination, alternate, route, remarks, runway, squawk,
                                   assigned_squawk, sid, cleared, ground_state, cleared_altitude, requested_altitude,
                                   heading, aircraft_type, aircraft_category, position, stand, communication_type,
                                   capabilities, eobt, eldt, tracking_controller, engine_type, has_fp, type);

};

struct ControllerOnlineEvent final : Event {
    std::string callsign;
    std::string position;

    explicit ControllerOnlineEvent(std::string callsign, std::string position) : Event(EVENT_CONTROLLER_ONLINE),
        callsign(std::move(callsign)), position(std::move(position)) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(ControllerOnlineEvent, callsign, position, type);
};

struct ControllerOfflineEvent final : Event {
    std::string callsign;

    explicit ControllerOfflineEvent(std::string callsign) : Event(EVENT_CONTROLLER_OFFLINE),
                                                            callsign(std::move(callsign)) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(ControllerOfflineEvent, callsign, type);
};

struct StandEvent final : Event {
    std::string callsign;
    std::string stand;

    explicit StandEvent(std::string callsign, std::string stand) : Event(EVENT_STAND),
                                                                   callsign(std::move(callsign)),
                                                                   stand(std::move(stand)) {
    }
    StandEvent() = default;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(StandEvent, callsign, stand, type);
};

struct TrackingControllerChangedEvent final : Event {
    std::string callsign;
    std::string tracking_controller;

    TrackingControllerChangedEvent(std::string callsign, std::string trackingController)
        : Event(EVENT_TRACKING_CONTROLLER_CHANGED),
          callsign(std::move(callsign)),
          tracking_controller(std::move(trackingController)) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(TrackingControllerChangedEvent, callsign, tracking_controller, type);
};


struct Strip final {
    Strip(std::string callsign, std::string origin, std::string destination, std::string alternate, std::string route,
          std::string remarks, std::string runway, std::string squawk, std::string assigned_squawk, std::string sid,
          bool cleared, std::string ground_state, int cleared_altitude, int requested_altitude, int heading,
          std::string aircraft_type, std::string aircraft_category, Position position, std::string stand,
          std::string communication_type, std::string capabilities, std::string eobt, std::string eldt,
          std::string tracking_controller, std::string engine_type, bool has_fp = true)
        : callsign(std::move(callsign)),
          origin(std::move(origin)),
          destination(std::move(destination)),
          alternate(std::move(alternate)),
          route(std::move(route)),
          remarks(std::move(remarks)),
          runway(std::move(runway)),
          squawk(std::move(squawk)),
          assigned_squawk(std::move(assigned_squawk)),
          sid(std::move(sid)),
          cleared(cleared),
          ground_state(std::move(ground_state)),
          cleared_altitude(cleared_altitude),
          requested_altitude(requested_altitude),
          heading(heading),
          aircraft_type(std::move(aircraft_type)),
          aircraft_category(std::move(aircraft_category)),
          position(position),
          stand(std::move(stand)),
          communication_type(std::move(communication_type)),
          capabilities(std::move(capabilities)),
          eobt(std::move(eobt)),
          eldt(std::move(eldt)),
          tracking_controller(std::move(tracking_controller)),
          engine_type(std::move(engine_type)),
          has_fp(has_fp) {
    }

    std::string callsign;
    std::string origin;
    std::string destination;
    std::string alternate;
    std::string route;
    std::string remarks;
    std::string runway;
    std::string squawk;
    std::string assigned_squawk;
    std::string sid;
    bool cleared;
    std::string ground_state;
    int cleared_altitude;
    int requested_altitude;
    int heading;
    std::string aircraft_type;
    std::string aircraft_category;
    Position position;
    std::string stand;
    std::string communication_type;
    std::string capabilities;
    std::string eobt;
    std::string eldt;
    std::string tracking_controller;
    std::string engine_type;
    bool has_fp;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(Strip, callsign, origin, destination, alternate, route, remarks, runway, squawk,
                                   assigned_squawk, sid, cleared, ground_state, cleared_altitude, requested_altitude,
                                   heading, aircraft_type, aircraft_category, position, stand, communication_type,
                                   capabilities, eobt, eldt, tracking_controller, engine_type, has_fp);
};

struct Controller final {
    Controller(std::string position, std::string callsign)
        : position(std::move(position)),
          callsign(std::move(callsign)) {
    }

    std::string position;
    std::string callsign;


    NLOHMANN_DEFINE_TYPE_INTRUSIVE(Controller, position, callsign);
};


struct SyncEvent final : Event {
    SyncEvent(std::vector<Strip> strips, std::vector<Controller> controllers, std::vector<Runway> runways, std::vector<SidEntry> sids)
        : Event(EVENT_SYNC), strips(std::move(strips)),
          controllers(std::move(controllers)),
          runways(std::move(runways)),
          sids(std::move(sids)) {
    }

    std::vector<Strip> strips;
    std::vector<Controller> controllers;
    std::vector<Runway> runways;
    std::vector<SidEntry> sids;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(SyncEvent, strips, controllers, runways, sids, type);
};

/**
 * Server only events
 **/

struct SessionInfoEvent final : Event {
    std::string role;

    explicit SessionInfoEvent(std::string role) : Event(EVENT_SESSION_INFO),
                                                  role(std::move(role)) {
    }

    SessionInfoEvent() = default;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(SessionInfoEvent, role, type);
};

struct RunwayMismatchAlertEvent final : Event {
    std::vector<std::string> expected_departure;
    std::vector<std::string> expected_arrival;
    std::vector<std::string> current_departure;
    std::vector<std::string> current_arrival;

    RunwayMismatchAlertEvent() : Event(EVENT_RUNWAY_MISMATCH_ALERT) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(
        RunwayMismatchAlertEvent,
        expected_departure,
        expected_arrival,
        current_departure,
        current_arrival,
        type
    );
};

struct GenerateSquawkEvent final : Event {
    std::string callsign;

    explicit GenerateSquawkEvent(std::string callsign) : Event(EVENT_GENERATE_SQUAWK),
                                                         callsign(std::move(callsign)) {
    }
    GenerateSquawkEvent() = default;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(GenerateSquawkEvent, callsign, type);
};

struct RouteEvent final : Event {
    std::string callsign;
    std::string route;

    explicit RouteEvent(std::string callsign, std::string route) : Event(EVENT_ROUTE),
                                                                   callsign(std::move(callsign)),
                                                                   route(std::move(route)) {
    }
    RouteEvent() = default;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(RouteEvent, callsign, route, type);
};

struct RemarksEvent final : Event {
    std::string callsign;
    std::string remarks;

    explicit RemarksEvent(std::string callsign, std::string remarks) : Event(EVENT_REMARKS),
                                                                       callsign(std::move(callsign)),
                                                                       remarks(std::move(remarks)) {
    }
    RemarksEvent() = default;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(RemarksEvent, callsign, remarks, type);
};

struct SidEvent final : Event {
    std::string callsign;
    std::string sid;

    explicit SidEvent(std::string callsign, std::string sid) : Event(EVENT_SID),
                                                               callsign(std::move(callsign)), sid(std::move(sid)) {
    }
    SidEvent() = default;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(SidEvent, callsign, sid, type);
};

struct AircraftRunwayEvent final : Event {
    std::string callsign;
    std::string runway;

    explicit AircraftRunwayEvent(std::string callsign, std::string runway) : Event(EVENT_AIRCRAFT_RUNWAY),
                                                                             callsign(std::move(callsign)),
                                                                             runway(std::move(runway)) {
    }
    AircraftRunwayEvent() = default;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(AircraftRunwayEvent, callsign, runway, type);
};

struct CoordinationReceivedEvent final : Event {
    std::string callsign;
    std::string source_controller_callsign;
    std::string controller_callsign;

    CoordinationReceivedEvent(std::string callsign, std::string sourceControllerCallsign, std::string controllerCallsign)
        : Event(EVENT_COORDINATION_RECEIVED),
          callsign(std::move(callsign)),
          source_controller_callsign(std::move(sourceControllerCallsign)),
          controller_callsign(std::move(controllerCallsign)) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(CoordinationReceivedEvent, callsign, source_controller_callsign, controller_callsign, type);
};

struct AssumeAndDropEvent final : Event {
    std::string callsign;

    AssumeAndDropEvent() = default;
    explicit AssumeAndDropEvent(std::string callsign) : Event(EVENT_ASSUME_AND_DROP), callsign(std::move(callsign)) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(AssumeAndDropEvent, callsign, type);
};

struct AssumeOnlyEvent final : Event {
    std::string callsign;

    AssumeOnlyEvent() = default;
    explicit AssumeOnlyEvent(std::string callsign) : Event(EVENT_ASSUME_ONLY), callsign(std::move(callsign)) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(AssumeOnlyEvent, callsign, type);
};

struct DropTrackingEvent final : Event {
    std::string callsign;

    DropTrackingEvent() = default;
    explicit DropTrackingEvent(std::string callsign) : Event(EVENT_DROP_TRACKING), callsign(std::move(callsign)) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(DropTrackingEvent, callsign, type);
};

struct CoordinationHandoverEvent final : Event {
    std::string callsign;
    std::string target_callsign;

    CoordinationHandoverEvent(std::string callsign, std::string targetCallsign)
        : Event(EVENT_COORDINATION_HANDOVER),
          callsign(std::move(callsign)),
          target_callsign(std::move(targetCallsign)) {
    }
    CoordinationHandoverEvent() = default;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(CoordinationHandoverEvent, callsign, target_callsign, type);
};

struct BackendSyncStrip final {
    std::string callsign;
    std::string assigned_squawk;
    bool cleared;
    std::string ground_state;
    std::string stand;
    BackendSyncCdmData cdm{};
    std::string pdc_state{};
    std::string pdc_request_remarks{};

    BackendSyncStrip() = default;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE_WITH_DEFAULT(BackendSyncStrip, callsign, assigned_squawk, cleared, ground_state, stand, cdm, pdc_state, pdc_request_remarks);
};

struct BackendSyncEvent final : Event {
    std::vector<BackendSyncStrip> strips;
    double latitude = 0.0;
    double longitude = 0.0;

    BackendSyncEvent() = default;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE_WITH_DEFAULT(BackendSyncEvent, strips, type, latitude, longitude);
};

struct CreateFPLEvent final : Event {
    std::string callsign;
    std::string origin;
    std::string destination;
    std::string alternate_ad;
    std::string sid;
    std::string assigned_squawk;
    std::string eobt;
    std::string aircraft_type;
    int requested_altitude = 0;
    std::string route;
    std::string stand;
    std::string runway;
    std::string remarks;
    int persons_on_board = 0;
    std::string fpl_type;
    std::string language;

    CreateFPLEvent() = default;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(CreateFPLEvent, callsign, origin, destination, alternate_ad, sid, assigned_squawk,
                                   eobt, aircraft_type, requested_altitude, route, stand, runway, remarks,
                                   persons_on_board, fpl_type, language, type);
};

struct PdcStateChangeEvent final : Event {
    std::string callsign;
    std::string state;
    std::string pdc_request_remarks{};

    PdcStateChangeEvent() = default;
    PdcStateChangeEvent(std::string callsign, std::string state, std::string pdc_request_remarks = {})
        : Event(EVENT_PDC_STATE_CHANGE), callsign(std::move(callsign)), state(std::move(state)),
          pdc_request_remarks(std::move(pdc_request_remarks)) {}

    NLOHMANN_DEFINE_TYPE_INTRUSIVE_WITH_DEFAULT(PdcStateChangeEvent, callsign, state, pdc_request_remarks, type);
};

struct IssuePdcClearanceEvent final : Event {
    std::string callsign;
    std::string remarks;

    IssuePdcClearanceEvent() = default;
    explicit IssuePdcClearanceEvent(std::string callsign, std::string remarks = {})
        : Event(EVENT_ISSUE_PDC_CLEARANCE), callsign(std::move(callsign)), remarks(std::move(remarks)) {}

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(IssuePdcClearanceEvent, callsign, remarks, type);
};

struct PdcRevertToVoiceEvent final : Event {
    std::string callsign;

    PdcRevertToVoiceEvent() = default;
    explicit PdcRevertToVoiceEvent(std::string callsign)
        : Event(EVENT_PDC_REVERT_TO_VOICE), callsign(std::move(callsign)) {}

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(PdcRevertToVoiceEvent, callsign, type);
};

#endif //EVENTS_H
