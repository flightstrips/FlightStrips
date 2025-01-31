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
    EventType type { EVENT_UNKNOWN };

    explicit Event(const EventType type) : type(type) {}
};

struct TokenEvent final : Event {
    std::string token;
    explicit TokenEvent(std::string token) : Event(EVENT_TOKEN), token(std::move(token)) {}
    NLOHMANN_DEFINE_TYPE_INTRUSIVE(TokenEvent, token, type)
};

#endif //EVENTS_H