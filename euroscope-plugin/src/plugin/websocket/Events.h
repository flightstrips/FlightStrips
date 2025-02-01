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
                                                                  callsign(std::move(callsign)), squawk(std::move(squawk)) {
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

#endif //EVENTS_H
