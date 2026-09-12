#include <gtest/gtest.h>

#include "websocket/ProtoCodec.h"
#include "websocket/generated/proto/euroscope.pb.h"

using namespace FlightStrips::websocket;

TEST(ProtoCodecTest, SerializesTokenAsOneofEnvelope) {
    const TokenEvent event("access-token", "2.4.0");

    const auto bytes = protobuf::Serialize(event);
    protobuf::wire::Envelope envelope;

    ASSERT_TRUE(protobuf::ParseEnvelope(bytes, envelope));
    EXPECT_EQ(envelope.event_case(), protobuf::wire::Envelope::kToken);
    EXPECT_EQ(envelope.token().token(), "access-token");
    EXPECT_EQ(envelope.token().version(), "2.4.0");
    EXPECT_EQ(protobuf::GetEventType(envelope), EVENT_TOKEN);
}

TEST(ProtoCodecTest, SerializesLoginPayloadWithoutASeparateTypeHeader) {
    const LoginEvent event("EKCH", "VATSIM", "TWR", "EKCH_TWR", 100, false, "127.0.0.1");

    const auto bytes = protobuf::Serialize(event);
    protobuf::wire::Envelope envelope;

    ASSERT_TRUE(envelope.ParseFromString(bytes));
    ASSERT_TRUE(envelope.has_login());
    EXPECT_EQ(envelope.login().airport(), "EKCH");
    EXPECT_EQ(envelope.login().callsign(), "EKCH_TWR");
    EXPECT_EQ(envelope.login().range(), 100);
    EXPECT_EQ(envelope.login().local_ip(), "127.0.0.1");
}

TEST(ProtoCodecTest, SerializesAssignedSpeedAsTypedOneof) {
    const AMANRouteFactEvent event("SAS123", AMANAssignedSpeed{std::nullopt, 750}, "2026-08-20T11:59:00Z");

    const auto bytes = protobuf::Serialize(event);
    protobuf::wire::Envelope envelope;

    ASSERT_TRUE(protobuf::ParseEnvelope(bytes, envelope));
    ASSERT_TRUE(envelope.aman_route_fact().data().has_assigned_speed());
    const auto& speed = envelope.aman_route_fact().data().assigned_speed();
    EXPECT_EQ(speed.value_case(), protobuf::wire::AssignedSpeed::kMachThousandths);
    EXPECT_EQ(speed.mach_thousandths(), 750);
}

TEST(ProtoCodecTest, DecodesBackendEventFromOneofPayload) {
    protobuf::wire::Envelope envelope;
    auto* payload = envelope.mutable_cdm_update();
    payload->set_callsign("SAS123");
    payload->set_tobt("1200");
    payload->set_tsat("1205");
    payload->set_ctot("1210");
    auto* restriction = payload->add_ecfmp_restrictions();
    restriction->set_measure_id(42);
    restriction->set_type("mandatory_route");
    restriction->add_routes("VEDAR DCT");

    CdmUpdateEvent event;
    protobuf::Decode(envelope.cdm_update(), event);

    EXPECT_EQ(envelope.event_case(), protobuf::wire::Envelope::kCdmUpdate);
    EXPECT_EQ(event.callsign, "SAS123");
    EXPECT_EQ(event.tobt, "1200");
    EXPECT_EQ(event.tsat, "1205");
    EXPECT_EQ(event.ctot, "1210");
    ASSERT_EQ(event.ecfmp_restrictions.size(), 1);
    EXPECT_EQ(event.ecfmp_restrictions[0].measure_id, 42);
    EXPECT_EQ(event.ecfmp_restrictions[0].type, "mandatory_route");
    EXPECT_EQ(event.ecfmp_restrictions[0].routes, std::vector<std::string>{"VEDAR DCT"});
}
