#ifndef EVENTS_H
#define EVENTS_H
#include <nlohmann/json.hpp>
#include <utility>

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
    EVENT_STRIP_UPDATE,
    EVENT_RUNWAY,
    // Server only events:
    EVENT_SESSION_INFO,
    EVENT_GENERATE_SQUAWK,
    EVENT_ROUTE,
    EVENT_REMARKS,
    EVENT_SID,
    EVENT_AIRCRAFT_RUNWAY,
};

NLOHMANN_JSON_SERIALIZE_ENUM(EventType, {
                             {EVENT_UNKNOWN, "unknown"},
                             {EVENT_TOKEN, "token"},
                             {EVENT_LOGIN, "login"},
                             {EVENT_CONTROLLER_ONLINE, "controller_online"},
                             {EVENT_CONTROLLER_OFFLINE, "controller_offline"},
                             {EVENT_SYNC, "sync"},
                             {EVENT_ASSIGNED_SQUAWK, "assigned_squawk"},
                             {EVENT_SQUAWK, "squawk"},
                             {EVENT_REQUESTED_ALTITUDE, "requested_altitude"},
                             {EVENT_CLEARED_ALTITUDE, "cleared_altitude"},
                             {EVENT_COMMUNICATION_TYPE, "communication_type"},
                             {EVENT_GROUND_STATE, "ground_state"},
                             {EVENT_CLEARED_FLAG, "cleared_flag"},
                             {EVENT_AIRCRAFT_POSITION_UPDATE, "aircraft_position_update"},
                             {EVENT_HEADING, "heading"},
                             {EVENT_AIRCRAFT_DISCONNECT, "aircraft_disconnect"},
                             {EVENT_STAND, "stand"},
                             {EVENT_STRIP_UPDATE, "strip_update"},
                             {EVENT_RUNWAY, "runway"},
                             {EVENT_SESSION_INFO, "session_info"},
                             {EVENT_GENERATE_SQUAWK, "generate_squawk"},
                             {EVENT_ROUTE, "route"},
                             {EVENT_REMARKS, "remarks"},
                             {EVENT_SID, "sid"},
                             {EVENT_AIRCRAFT_RUNWAY, "aircraft_runway"},
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
    std::string position;
    std::string callsign;
    int range;

    LoginEvent(std::string airport, std::string position, std::string callsign, const int range)
        : Event(EVENT_LOGIN),
          airport(std::move(airport)),
          position(std::move(position)),
          callsign(std::move(callsign)),
          range(range) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(LoginEvent, airport, position, callsign, range, type);
};

struct Runway final {
    std::string name;
    bool departure;
    bool arrival;
    NLOHMANN_DEFINE_TYPE_INTRUSIVE(Runway, name, departure, arrival);
};

struct RunwayEvent final : Event {
    std::vector<Runway> runways;

    explicit RunwayEvent(std::vector<Runway> runways) : Event(EVENT_RUNWAY), runways(std::move(runways)) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(RunwayEvent, runways, type);
};

struct AssignedSquawkEvent final : Event {
    std::string callsign;
    std::string squawk;

    AssignedSquawkEvent(std::string callsign, std::string squawk) : Event(EVENT_ASSIGNED_SQUAWK),
                                                                    callsign(std::move(callsign)),
                                                                    squawk(std::move(squawk)) {
    }

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

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(HeadingEvent, callsign, heading, type);
};

struct RequestedAltitudeEvent final : Event {
    std::string callsign;
    int altitude;

    explicit RequestedAltitudeEvent(std::string callsign, const int altitude) : Event(EVENT_REQUESTED_ALTITUDE),
        callsign(std::move(callsign)), altitude(altitude) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(RequestedAltitudeEvent, callsign, altitude, type);
};

struct ClearedAltitudeEvent final : Event {
    std::string callsign;
    int altitude;

    explicit ClearedAltitudeEvent(std::string callsign, const int altitude) : Event(EVENT_CLEARED_ALTITUDE),
                                                                              callsign(std::move(callsign)),
                                                                              altitude(altitude) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(ClearedAltitudeEvent, callsign, altitude, type);
};

struct CommunicationTypeEvent final : Event {
    std::string callsign;
    std::string communication_type;

    explicit
    CommunicationTypeEvent(std::string callsign, const char communicationType) : Event(EVENT_COMMUNICATION_TYPE),
        callsign(std::move(callsign)), communication_type({communicationType}) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(CommunicationTypeEvent, callsign, communication_type, type);
};

struct GroundStateEvent final : Event {
    std::string callsign;
    std::string ground_state;

    explicit GroundStateEvent(std::string callsign, std::string groundState) : Event(EVENT_GROUND_STATE),
                                                                               callsign(std::move(callsign)),
                                                                               ground_state(std::move(groundState)) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(GroundStateEvent, callsign, ground_state, type);
};

struct ClearedFlagEvent final : Event {
    std::string callsign;
    bool cleared;

    explicit ClearedFlagEvent(std::string callsign, const bool cleared) : Event(EVENT_CLEARED_FLAG),
                                                                          callsign(std::move(callsign)),
                                                                          cleared(cleared) {
    }

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

struct StripUpdateEvent final : Event {
    StripUpdateEvent(std::string callsign, std::string origin, std::string destination, std::string alternate,
                     std::string route, std::string remarks, std::string runway, std::string sid,
                     std::string aircraft_type,
                     std::string aircraft_category, std::string capabilities, std::string eobt, std::string eldt)
        : Event(EVENT_STRIP_UPDATE),
          callsign(std::move(callsign)),
          origin(std::move(origin)),
          destination(std::move(destination)),
          alternate(std::move(alternate)),
          route(std::move(route)),
          remarks(std::move(remarks)),
          runway(std::move(runway)),
          sid(std::move(sid)),
          aircraft_type(std::move(aircraft_type)),
          aircraft_category(std::move(aircraft_category)),
          capabilities(std::move(capabilities)),
          eobt(std::move(eobt)),
          eldt(std::move(eldt)) {
    }

    std::string callsign;
    std::string origin;
    std::string destination;
    std::string alternate;
    std::string route;
    std::string remarks;
    std::string runway;
    std::string sid;
    std::string aircraft_type;
    std::string aircraft_category;
    std::string capabilities;
    std::string eobt;
    std::string eldt;


    NLOHMANN_DEFINE_TYPE_INTRUSIVE(StripUpdateEvent, callsign, origin, destination, alternate, route, remarks, runway,
                                   sid, aircraft_type, aircraft_category, capabilities, eobt, eldt, type);
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

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(StandEvent, callsign, stand, type);
};

struct Position final {
    double lat;
    double lon;
    int altitude;

    explicit Position(const double lat, const double lon, const int altitude) : lat(lat), lon(lon), altitude(altitude) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(Position, lat, lon, altitude);
};

struct Strip final {
    Strip(std::string callsign, std::string origin, std::string destination, std::string alternate, std::string route,
          std::string remarks, std::string runway, std::string squawk, std::string assigned_squawk, std::string sid,
          bool cleared, std::string ground_state, int cleared_altitude, int requested_altitude, int heading,
          std::string aircraft_type, std::string aircraft_category, Position position, std::string stand,
          std::string communication_type, std::string capabilities, std::string eobt, std::string eldt)
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
          eldt(std::move(eldt)) {
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

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(Strip, callsign, origin, destination, alternate, route, remarks, runway, squawk,
                                   assigned_squawk, sid, cleared, ground_state, cleared_altitude, requested_altitude,
                                   heading, aircraft_type, aircraft_category, position, stand, communication_type,
                                   capabilities, eobt, eldt);
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
    SyncEvent(std::vector<Strip> strips, std::vector<Controller> controllers)
        : Event(EVENT_SYNC), strips(std::move(strips)),
          controllers(std::move(controllers)) {
    }

    std::vector<Strip> strips;
    std::vector<Controller> controllers;

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(SyncEvent, strips, controllers, type);
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

struct GenerateSquawkEvent final : Event {
    std::string callsign;

    explicit GenerateSquawkEvent(std::string callsign) : Event(EVENT_GENERATE_SQUAWK),
                                                         callsign(std::move(callsign)) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(GenerateSquawkEvent, callsign, type);
};

struct RouteEvent final : Event {
    std::string callsign;
    std::string route;

    explicit RouteEvent(std::string callsign, std::string route) : Event(EVENT_ROUTE),
                                                                   callsign(std::move(callsign)),
                                                                   route(std::move(route)) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(RouteEvent, callsign, route, type);
};

struct RemarksEvent final : Event {
    std::string callsign;
    std::string remarks;

    explicit RemarksEvent(std::string callsign, std::string remarks) : Event(EVENT_REMARKS),
                                                                       callsign(std::move(callsign)),
                                                                       remarks(std::move(remarks)) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(RemarksEvent, callsign, remarks, type);
};

struct SidEvent final : Event {
    std::string callsign;
    std::string sid;

    explicit SidEvent(std::string callsign, std::string sid) : Event(EVENT_SID),
                                                               callsign(std::move(callsign)), sid(std::move(sid)) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(SidEvent, callsign, sid, type);
};

struct AircraftRunwayEvent final : Event {
    std::string callsign;
    std::string runway;

    explicit AircraftRunwayEvent(std::string callsign, std::string runway) : Event(EVENT_AIRCRAFT_RUNWAY),
                                                                             callsign(std::move(callsign)),
                                                                             runway(std::move(runway)) {
    }

    NLOHMANN_DEFINE_TYPE_INTRUSIVE(AircraftRunwayEvent, callsign, runway, type);
};

#endif //EVENTS_H
