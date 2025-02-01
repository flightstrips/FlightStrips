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
    EVENT_RUNWAY
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
                             })

struct Event {
    EventType type{EVENT_UNKNOWN};

    explicit Event(const EventType type) : type(type) {
    }
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

#endif //EVENTS_H
