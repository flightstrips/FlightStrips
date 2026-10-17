#include <gtest/gtest.h>

#include "websocket/ProtoCodec.h"
#include "websocket/generated/proto/euroscope.pb.h"

using namespace FlightStrips::websocket;

TEST(ProtoCodecTest, SerializesFiledFlightRules) {
    StripUpdateEvent event(
        "OYABC", "EKRK", "EKCH", "", "", "", "", "7000", "7000", "", "",
        false, "", 0, 0, 0, "C172", "L", "", Position{0, 0, 1000},
        "", "V", "", "", "", "", "");
    event.flight_rules = "V";
    protobuf::wire::Envelope envelope;
    ASSERT_TRUE(protobuf::ParseEnvelope(protobuf::Serialize(event), envelope));
    EXPECT_EQ(envelope.strip_update().strip().flight_rules(), "V");
}

TEST(ProtoCodecTest, SharedScratchPadRoundTripsEditAndClear) {
    for (const auto& text : {"CALL OPS", ""}) {
        protobuf::wire::Envelope envelope;
        ASSERT_TRUE(protobuf::ParseEnvelope(protobuf::Serialize(FsScratchPadEvent{"SAS123", text}), envelope));
        EXPECT_EQ(protobuf::GetEventType(envelope), EVENT_FS_SCRATCH_PAD);
        FsScratchPadEvent decoded;
        protobuf::Decode(envelope.fs_scratch_pad(), decoded);
        EXPECT_EQ(decoded.callsign, "SAS123");
        EXPECT_EQ(decoded.text, text);
    }
}

TEST(ProtoCodecTest, DecodesTrackingOwnershipConfirmation) {
    protobuf::wire::TrackingControllerChangedEvent source;
    source.set_callsign("SAS123");
    source.set_tracking_controller("EKCH_B_APP");
    TrackingControllerChangedEvent event;
    protobuf::Decode(source, event);
    EXPECT_EQ(event.callsign, "SAS123");
    EXPECT_EQ(event.tracking_controller, "EKCH_B_APP");
}

TEST(ProtoCodecTest, SerializesTokenAsOneofEnvelope) {
    const TokenEvent event("access-token", "2.4.0");

    const auto bytes = protobuf::Serialize(event);
    protobuf::wire::Envelope envelope;

    ASSERT_TRUE(protobuf::ParseEnvelope(bytes, envelope));
    EXPECT_EQ(envelope.event_case(), protobuf::wire::Envelope::kToken);
    EXPECT_EQ(envelope.token().token(), "access-token");
    EXPECT_EQ(envelope.token().version(), "2.4.0");
    EXPECT_TRUE(envelope.token().frequency_messages());
    EXPECT_EQ(protobuf::GetEventType(envelope), EVENT_TOKEN);
}

TEST(ProtoCodecTest, DecodesFrequencyMessageAndAcknowledgesSubmission) {
    protobuf::wire::SendFrequencyMessageEvent source;
    source.set_request_id("request-1"); source.set_callsign("SAS123");
    source.set_message("Taxi via TWY A."); source.set_primary_frequency("121.905"); source.set_applied_actions(2);
    SendFrequencyMessageEvent decoded;
    protobuf::Decode(source, decoded);
    EXPECT_EQ(decoded.request_id, "request-1"); EXPECT_EQ(decoded.callsign, "SAS123");
    EXPECT_EQ(decoded.message, "Taxi via TWY A."); EXPECT_EQ(decoded.primary_frequency, "121.905"); EXPECT_EQ(decoded.applied_actions, 2);
    FrequencyMessageResultEvent result;
    result.request_id = decoded.request_id; result.callsign = decoded.callsign;
    result.applied_actions = decoded.applied_actions; result.submitted = false; result.error = "Command input busy";
    protobuf::wire::Envelope envelope;
    ASSERT_TRUE(protobuf::ParseEnvelope(protobuf::Serialize(result), envelope));
    EXPECT_EQ(protobuf::GetEventType(envelope), EVENT_FREQUENCY_MESSAGE_RESULT);
    EXPECT_EQ(envelope.frequency_message_result().request_id(), "request-1");
    EXPECT_EQ(envelope.frequency_message_result().applied_actions(), 2);
    EXPECT_FALSE(envelope.frequency_message_result().submitted());
    EXPECT_EQ(envelope.frequency_message_result().error(), "Command input busy");
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

TEST(ProtoCodecTest, DecodesBackendHoldingEatWriteback) {
	protobuf::wire::Envelope envelope;
	auto* payload = envelope.mutable_hold();
	payload->set_callsign("SAS123");
	payload->set_hold("OLPIB");
	payload->set_hold_type("enroute");
	payload->set_hold_eat("1422");

	HoldEvent event;
	protobuf::Decode(envelope.hold(), event);

	EXPECT_EQ(event.callsign, "SAS123");
	EXPECT_EQ(event.hold, "OLPIB");
	EXPECT_EQ(event.hold_type, "enroute");
	EXPECT_EQ(event.hold_eat, "1422");
}

TEST(ProtoCodecTest, DecodesPersistedHoldFromBackendSync) {
    protobuf::wire::BackendSyncEvent source;
    auto* strip = source.add_strips();
    strip->set_callsign("SAS123");
    strip->set_hold("OLPIB");
    strip->set_hold_type("enroute");
    strip->set_hold_eat("1422");

    BackendSyncEvent event;
    protobuf::Decode(source, event);

    ASSERT_EQ(event.strips.size(), 1u);
    EXPECT_EQ(event.strips[0].callsign, "SAS123");
    EXPECT_EQ(event.strips[0].hold, "OLPIB");
    EXPECT_EQ(event.strips[0].hold_type, "enroute");
    EXPECT_EQ(event.strips[0].hold_eat, "1422");
}
