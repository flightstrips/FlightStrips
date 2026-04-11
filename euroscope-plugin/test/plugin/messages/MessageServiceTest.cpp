#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "messages/MessageService.h"
#include "websocket/Events.h"

// MessageService constructor requires shared_ptr<FlightStripsPlugin> and
// several other heavyweight services that depend on Win32/EuroScope
// infrastructure unavailable in unit tests.
//
// We therefore only compile-test the public interface (header inclusion,
// type availability) and document what would be covered with proper seams.
//
// If MessageService were refactored to accept lightweight interfaces, the
// following behaviours should be tested per message type:
//
//  - "assigned_squawk"      → GetFlightPlan; SetSquawk
//  - "ground_state"         → SetGroundState
//  - "stand"                → SetStand
//  - "sid"                  → SetSid via RouteService
//  - "aircraft_runway"      → SetRunway via RouteService
//  - "session_info"         → stores session info
//  - ... (all HandleXxxEvent helpers)

TEST(MessageServiceTest, HeaderCompiles) {
    using FlightStrips::messages::MessageService;
    SUCCEED();
}

TEST(MessageServiceEventsTest, CdmTobtUpdateEventSerializesExpectedShape) {
    const nlohmann::json json = CdmTobtUpdateEvent{"EIN123", "1030"};
    EXPECT_EQ(json.at("type").get<std::string>(), EVENT_CDM_TOBT_UPDATE_NAME);
    EXPECT_EQ(json.at("callsign").get<std::string>(), "EIN123");
    EXPECT_EQ(json.at("tobt").get<std::string>(), "1030");
}

TEST(MessageServiceEventsTest, CdmUpdateEventDeserializesRequestedTobtSourceAndFlowMessage) {
    const auto json = nlohmann::json::parse(R"({
        "type":"cdm_update",
        "callsign":"EIN123",
        "req_tobt":"1025",
        "req_tobt_source":"PILOT",
        "asat":"1031",
        "deice_type":"M",
        "ecfmp_id":"REGUL"
    })");

    const auto event = json.get<CdmUpdateEvent>();
    EXPECT_EQ(event.callsign, "EIN123");
    EXPECT_EQ(event.req_tobt, "1025");
    EXPECT_EQ(event.req_tobt_source, "PILOT");
    EXPECT_EQ(event.asat, "1031");
    EXPECT_EQ(event.deice_type, "M");
    EXPECT_EQ(event.ecfmp_id, "REGUL");
}

TEST(MessageServiceEventsTest, CdmTsacUpdateEventSerializesExpectedShape) {
    const nlohmann::json json = CdmTsacUpdateEvent{"EIN123", "1030"};
    EXPECT_EQ(json.at("type").get<std::string>(), EVENT_CDM_TSAC_UPDATE_NAME);
    EXPECT_EQ(json.at("callsign").get<std::string>(), "EIN123");
    EXPECT_EQ(json.at("tsac").get<std::string>(), "1030");
}

TEST(MessageServiceEventsTest, AssumeOnlyEventSerializesExpectedShape) {
    const nlohmann::json json = AssumeOnlyEvent{"EIN123"};
    EXPECT_EQ(json.at("type").get<std::string>(), EVENT_ASSUME_ONLY_NAME);
    EXPECT_EQ(json.at("callsign").get<std::string>(), "EIN123");
}

TEST(MessageServiceEventsTest, DropTrackingEventSerializesExpectedShape) {
    const nlohmann::json json = DropTrackingEvent{"EIN123"};
    EXPECT_EQ(json.at("type").get<std::string>(), EVENT_DROP_TRACKING_NAME);
    EXPECT_EQ(json.at("callsign").get<std::string>(), "EIN123");
}
