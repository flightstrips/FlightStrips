#include <gtest/gtest.h>
#include <algorithm>
#include <atomic>
#include <thread>
#include "vacs/VacsClient.h"
#include "vacs/VacsService.h"
#include "vacs/VacsRelay.h"

using namespace FlightStrips::vacs;

class VacsClientTest : public testing::Test {
protected:
    Time now{};
    std::vector<Json> sent;
    VacsClient client{[this](const std::string& message) { sent.push_back(Json::parse(message)); }};
    Json snapshot = {
        {"connectionState", "connected"}, {"clientId", "self"},
        {"sessionInfo", {{"client", {{"id", "self"}, {"positionId", "EKCH_TWR"}}}}},
        {"clients", Json::array({{{"id", "peer"}, {"displayName", "Approach"}, {"positionId", "EKCH_APP"}, {"frequency", "119.800"}}})},
        {"defaultCallSources", Json::array()}
    };
    void Reply(const Json& request, Json data = nullptr, bool ok = true) {
        client.Receive(Json{{"type", "response"}, {"id", request["id"]}, {"ok", ok}, {"data", data}, {"error", {{"detail", "rejected"}}}}.dump(), now);
    }
    void Event(const std::string& name, Json payload) { client.Receive(Json{{"type", "event"}, {"name", name}, {"payload", payload}}.dump(), now); }
    void Store(Json state) {
        if (state.is_object() && !state.contains("incomingCalls")) state["incomingCalls"] = nullptr;
        Event("store:sync", {{"store", "call"}, {"state", state}});
    }
    Json CallData(const std::string& id, const std::string& source = "peer", const std::string& target = "self") {
        return {{"callId", id}, {"source", {{"clientId", source}}}, {"target", {{"client", target}}},
                {"invitedTargets", Json::array()}, {"joinedParticipants", Json::object()}, {"prio", false}};
    }
    void Boot(bool recover = true) {
        client.Open(now);
        Reply(sent.back());
        Reply(sent.back(), snapshot);
        Reply(sent.back());
        if (recover) Store({{"prio", false}, {"incomingCalls", Json::array()}});
    }
    void Invite(const std::string& id) { Event("signaling:call-invitation", CallData(id)); }
    void PeerEvent(const std::string& name, const std::string& id, const std::string& peer = "peer") {
        Event("webrtc:call-" + name, {{"callId", id}, {"peerId", peer}});
    }
    void Connected(const std::string& id = "active") {
        auto call = CallData(id);
        call["joinedParticipants"] = {{"self", {{"target", {{"client", "self"}}}, {"state", "connected"}}},
                                       {"peer", {{"target", {{"client", "peer"}}}, {"state", "connected"}}}};
        Store({{"prio", false}, {"incomingCalls", Json::array()}, {"callDisplay", {{"type", "accepted"}, {"call", call}}}});
    }
};

TEST_F(VacsClientTest, RequiresDesktopStoreRecoveryBeforeIdle) {
    Boot(false);
    EXPECT_EQ(client.GetView().state, State::Synchronizing);
    Store({{"prio", false}, {"callDisplay", nullptr}});
    EXPECT_EQ(client.GetView().state, State::Synchronizing);
    Store({{"prio", false}, {"incomingCalls", Json::array()}});
    EXPECT_EQ(client.GetView().state, State::Idle);
}
TEST_F(VacsClientTest, ReportsVersionWithoutGatingBootstrapAndOnlyRequestsUntilLogged) {
    Boot();
    EXPECT_EQ(client.GetView().state, State::Idle);
    const auto version = *std::find_if(sent.begin(), sent.end(), [](const Json& message) {
        return message.value("cmd", "") == "app_get_version";
    });
    Reply(version, "3.0.1");
    EXPECT_TRUE(client.TakeError().empty());
    sent.clear();
    Boot();
    EXPECT_EQ(client.GetView().state, State::Idle);
    EXPECT_TRUE(std::none_of(sent.begin(), sent.end(), [](const Json& message) {
        return message.value("cmd", "") == "app_get_version";
    }));
}
TEST_F(VacsClientTest, UnavailableVersionDoesNotPreventReadinessOrReportError) {
    Boot();
    const auto version = *std::find_if(sent.begin(), sent.end(), [](const Json& message) {
        return message.value("cmd", "") == "app_get_version";
    });
    Reply(version, nullptr, false);
    EXPECT_EQ(client.GetView().state, State::Idle);
    EXPECT_TRUE(client.TakeError().empty());
}
TEST_F(VacsClientTest, SubscribesBeforeBootstrapAndPingsAtTwentySeconds) {
    Boot();
    EXPECT_EQ(sent.front()["type"], "subscribe");
    now += std::chrono::seconds(20); client.Tick(now);
    EXPECT_EQ(sent.back()["type"], "ping");
    client.Close(false); const auto count = sent.size();
    client.Tick(now + std::chrono::seconds(20)); EXPECT_EQ(sent.size(), count);
}
TEST_F(VacsClientTest, DialUsesLowercaseTargetAndDoesNotFabricateStation) {
    Boot(); ASSERT_TRUE(client.Execute(Action::Call, "", now, "peer"));
    const auto request = sent.back();
    EXPECT_EQ(request["cmd"], "signaling_invite_to_call");
    EXPECT_EQ(request["args"]["targets"], Json::array({{{"client", "peer"}}}));
    EXPECT_FALSE(request["args"].contains("target"));
    EXPECT_EQ(request["args"]["source"]["positionId"], "EKCH_TWR");
    EXPECT_FALSE(request["args"]["source"].contains("stationId"));
    EXPECT_TRUE(client.GetView().pending);
    EXPECT_FALSE(client.Execute(Action::Call, "", now, "peer"));
    Reply(request, "outgoing");
    EXPECT_EQ(client.GetView().state, State::Outgoing);
    EXPECT_EQ(client.GetView().callId, "outgoing");
}
TEST_F(VacsClientTest, UsesLiveDefaultStationSource) {
    snapshot["defaultCallSources"] = Json::array({"station"}); Boot();
    ASSERT_TRUE(client.Execute(Action::Call, "", now, "peer"));
    EXPECT_EQ(sent.back()["args"]["source"]["stationId"], "station");
}
TEST_F(VacsClientTest, RestoresConnectedCallAndNullPartialPreservesIt) {
    Boot(false); Connected();
    EXPECT_EQ(client.GetView().state, State::Connected);
    Store({{"prio", true}, {"callDisplay", nullptr}});
    EXPECT_EQ(client.GetView().callId, "active");
    EXPECT_EQ(client.GetView().state, State::Connected);
}
TEST_F(VacsClientTest, OmittedDisplayClearsActiveButPreservesIncomingQueue) {
    Boot(); Connected(); Invite("waiting");
    Store({{"prio", false}});
    EXPECT_EQ(client.GetView().state, State::Incoming);
    EXPECT_EQ(client.GetView().callId, "waiting");
}
TEST_F(VacsClientTest, MalformedAndUnrelatedStoreMessagesDoNotClearCalls) {
    Boot(); Connected();
    Event("store:sync", {{"store", "settings"}, {"state", {{"prio", false}}}});
    Store({{"prio", "false"}}); Store(Json::array()); Store({{"callDisplay", nullptr}});
    client.Receive("not json", now); client.Receive("null", now);
    EXPECT_EQ(client.GetView().callId, "active");
}
TEST_F(VacsClientTest, DeduplicatesIncomingAndAnswersOldestUntilWebRtcConfirms) {
    Boot(); Invite("first"); Invite("second"); Invite("first");
    EXPECT_EQ(client.GetView().waiting, 2);
    EXPECT_EQ(client.GetView().mini, "EKCH_APP (2)");
    ASSERT_TRUE(client.Execute(Action::Answer, "first", now));
    EXPECT_EQ(sent.back()["args"]["callId"], "first"); Reply(sent.back());
    EXPECT_EQ(client.GetView().state, State::Connecting);
    EXPECT_EQ(client.GetView().waiting, 1);
    PeerEvent("connected", "first");
    EXPECT_EQ(client.GetView().state, State::Connected);
    Event("signaling:call-end", "first");
    EXPECT_EQ(client.GetView().callId, "second");
}
TEST_F(VacsClientTest, PreservesFullIncomingCallerThroughAnswerAndConnection) {
    snapshot["clients"][0]["positionId"] = "EKDK_UC_CTR";
    Boot(); Invite("first"); Invite("second");
    EXPECT_EQ(client.GetView().label, "EKDK_UC_CTR (2)");
    EXPECT_EQ(client.GetView().mini, "EKDK_UC_CTR (2)");
    EXPECT_NE(client.GetView().tooltip.find("Incoming from EKDK_UC_CTR"), std::string::npos);
    ASSERT_TRUE(client.Execute(Action::Answer, "first", now)); Reply(sent.back());
    EXPECT_EQ(client.GetView().state, State::Connecting);
    EXPECT_EQ(client.GetView().label, "EKDK_UC_CTR");
    EXPECT_EQ(client.GetView().mini, "EKDK_UC_CTR");
    PeerEvent("connected", "first");
    EXPECT_EQ(client.GetView().state, State::Connected);
    EXPECT_EQ(client.GetView().mini, "EKDK_UC_CTR");
}
TEST_F(VacsClientTest, PreservesFullOutgoingPeerWithoutClippingPrefixes) {
    snapshot["clients"][0]["positionId"] = "EKDK_UC_CTR";
    Boot(); ASSERT_TRUE(client.Execute(Action::Call, "", now, "peer"));
    Reply(sent.back(), "outgoing");
    EXPECT_EQ(client.GetView().label, "EKDK_UC_CTR");
    EXPECT_EQ(client.GetView().mini, "EKDK_UC_CTR");
    EXPECT_NE(client.GetView().tooltip.find("Calling EKDK_UC_CTR"), std::string::npos);
}
TEST_F(VacsClientTest, RejectOnlyRemovesOldestAfterConfirmation) {
    Boot(); Invite("first"); Invite("second");
    ASSERT_TRUE(client.Execute(Action::Reject, "first", now));
    EXPECT_EQ(client.GetView().waiting, 2); Reply(sent.back());
    EXPECT_EQ(client.GetView().callId, "second");
}
TEST_F(VacsClientTest, ActiveEndRemainsPrimaryWhileIncomingWaits) {
    Boot(); Connected(); Invite("waiting");
    EXPECT_EQ(client.GetView().state, State::Connected);
    EXPECT_FALSE(client.Execute(Action::Answer, "waiting", now));
    EXPECT_TRUE(client.Execute(Action::End, "active", now));
    EXPECT_EQ(sent.back()["args"]["callId"], "active");
}
TEST_F(VacsClientTest, FailedEndKeepsConfirmedCallAndCanRetry) {
    Boot(); Connected(); ASSERT_TRUE(client.Execute(Action::End, "active", now));
    Reply(sent.back(), nullptr, false);
    EXPECT_EQ(client.GetView().state, State::Connected);
    EXPECT_FALSE(client.TakeError().empty());
    EXPECT_TRUE(client.Execute(Action::End, "active", now));
}
TEST_F(VacsClientTest, TimeoutResynchronizesWithoutRetryingCall) {
    Boot(); ASSERT_TRUE(client.Execute(Action::Call, "", now, "peer"));
    const auto late = sent.back();
    now += std::chrono::seconds(10); client.Tick(now);
    EXPECT_EQ(sent.back()["cmd"], "remote_get_session_state");
    EXPECT_FALSE(client.Execute(Action::Call, "", now, "peer"));
    Reply(late, "late-call"); EXPECT_EQ(client.GetView().state, State::Synchronizing);
    Reply(sent.back(), snapshot); Reply(sent.back()); Store({{"prio", false}, {"incomingCalls", Json::array()}});
    EXPECT_EQ(client.GetView().state, State::Idle);
    EXPECT_EQ(std::count_if(sent.begin(), sent.end(), [](const Json& message) { return message.value("cmd", "") == "signaling_invite_to_call"; }), 1);
}
TEST_F(VacsClientTest, LateDialResponseCannotResurrectEndedCall) {
    Boot(); ASSERT_TRUE(client.Execute(Action::Call, "", now, "peer")); const auto request = sent.back();
    Event("signaling:call-end", "ended"); Reply(request, "ended");
    EXPECT_EQ(client.GetView().state, State::Idle);
}

TEST_F(VacsClientTest, MalformedDialSuccessRequiresResynchronizationBeforeAnotherDial) {
    Boot(); ASSERT_TRUE(client.Execute(Action::Call, "", now, "peer"));
    Reply(sent.back(), nullptr);
    EXPECT_EQ(client.GetView().state, State::Synchronizing);
    EXPECT_FALSE(client.Execute(Action::Call, "", now, "peer"));
    EXPECT_FALSE(client.TakeError().empty());
}
TEST_F(VacsClientTest, LateAcceptCannotDowngradeWebRtcConfirmation) {
    Boot(); Invite("first"); ASSERT_TRUE(client.Execute(Action::Answer, "first", now)); const auto request = sent.back();
    Event("signaling:accept-incoming-call", "first"); PeerEvent("connected", "first"); Reply(request);
    EXPECT_EQ(client.GetView().state, State::Connected);
}
TEST_F(VacsClientTest, OldGenerationReplyIgnoredAfterReconnect) {
    Boot(); ASSERT_TRUE(client.Execute(Action::Call, "", now, "peer")); const auto request = sent.back();
    Boot(); Reply(request, "old-call"); EXPECT_EQ(client.GetView().state, State::Idle);
}
TEST_F(VacsClientTest, StaleSnapshotCannotOverwriteNewerSessionEvent) {
    client.Open(now); Reply(sent.back()); const auto oldSnapshot = sent.back();
    Event("signaling:ambiguous-position", nullptr); Reply(oldSnapshot, snapshot);
    EXPECT_EQ(client.GetView().state, State::Synchronizing);
    EXPECT_EQ(sent.back()["cmd"], "remote_get_session_state");
}
TEST_F(VacsClientTest, ReadinessAndDisconnectedAuthenticationAreIndependent) {
    Boot(); Event("signaling:disconnected", nullptr);
    EXPECT_EQ(client.GetView().state, State::Disconnected);
    EXPECT_FALSE(client.Execute(Action::Call, "", now, "peer"));
    Event("auth:unauthenticated", nullptr);
    EXPECT_EQ(client.GetView().state, State::Unauthenticated);
}
TEST_F(VacsClientTest, StaleOrUnknownTargetsAndCallIdsNeverInvoke) {
    Boot(); EXPECT_FALSE(client.Execute(Action::Call, "", now, "missing"));
    Invite("first"); EXPECT_FALSE(client.Execute(Action::Reject, "stale", now));
    EXPECT_FALSE(client.Execute(Action::Cancel, "first", now));
}
TEST_F(VacsClientTest, RestoresIncomingAndOutgoingFromDesktopStore) {
    Boot(false);
    auto call = CallData("ringing", "self", "peer");
    call["invitedTargets"] = Json::array({{{"client", "peer"}}});
    Store({{"prio", false}, {"incomingCalls", Json::array({CallData("waiting")})}, {"callDisplay", {{"type", "outgoing"}, {"call", call}}}});
    EXPECT_EQ(client.GetView().state, State::Outgoing);
    EXPECT_EQ(client.GetView().waiting, 1);
    ASSERT_TRUE(client.Execute(Action::Cancel, "ringing", now)); Reply(sent.back());
    EXPECT_EQ(client.GetView().callId, "waiting");
}
TEST_F(VacsClientTest, PickerFiltersSelfSamePositionAndDuplicates) {
    snapshot["clients"].push_back({{"id", "self"}, {"positionId", "EKCH_TWR"}});
    snapshot["clients"].push_back({{"id", "other"}, {"positionId", "EKCH_TWR"}});
    snapshot["clients"].push_back(snapshot["clients"][0]); Boot();
    ASSERT_EQ(client.GetClients().size(), 1); EXPECT_EQ(client.GetClients()[0].id, "peer");
}
TEST_F(VacsClientTest, TransientWebRtcDisconnectKeepsCallEndable) {
    Boot(); Connected();
    PeerEvent("disconnected", "active");
    EXPECT_EQ(client.GetView().state, State::Connecting);
    EXPECT_EQ(client.GetView().callId, "active");
    EXPECT_NE(client.GetView().tooltip.find("Reconnecting to EKCH_APP"), std::string::npos);
    PeerEvent("connected", "active");
    EXPECT_EQ(client.GetView().state, State::Connected);
    Connected();
    EXPECT_EQ(client.GetView().callId, "active");
    EXPECT_TRUE(client.Execute(Action::End, "active", now));
}
TEST_F(VacsClientTest, DisconnectedCallRecoversFromConfirmedStoreAfterResync) {
    Boot(); Connected();
    PeerEvent("disconnected", "active");
    Event("signaling:connected", {{"client", {{"positionId", "EKCH_TWR"}}}});
    Reply(sent.back(), snapshot); Reply(sent.back()); Connected();
    EXPECT_EQ(client.GetView().state, State::Connected);
    EXPECT_EQ(client.GetView().callId, "active");
    EXPECT_TRUE(client.GetView().actionable);
}
TEST_F(VacsClientTest, RemoteDialUsesServerOutgoingEventWithoutWritingDesktopStore) {
    Boot(); ASSERT_TRUE(client.Execute(Action::Call, "", now, "peer"));
    const auto dial = sent.back(); Reply(dial, "outgoing");
    Event("signaling:outgoing-call", {{"callId", "outgoing"}, {"source", dial["args"]["source"]}, {"targets", dial["args"]["targets"]}, {"prio", false}});
    EXPECT_EQ(sent.back(), dial);
    EXPECT_EQ(client.GetView().state, State::Outgoing);
    EXPECT_TRUE(client.GetView().actionable);
}
TEST_F(VacsClientTest, RestoresDesktopOutgoingAndTransitionsThroughParticipantUpdate) {
    Boot(false);
    auto call = CallData("ringing", "self", "peer");
    call["invitedTargets"] = Json::array({{{"client", "peer"}}});
    Store({{"prio", true}, {"incomingCalls", Json::array()}, {"callDisplay", {{"type", "outgoing"}, {"call", call}}}});
    EXPECT_EQ(client.GetView().state, State::Outgoing);
    EXPECT_EQ(client.GetView().callId, "ringing");
    EXPECT_TRUE(client.GetView().actionable);
    Event("signaling:call-update", {{"callId", "ringing"}, {"invitedTargets", Json::array()},
                                    {"joinedParticipants", {{"peer", {{"client", "peer"}}}, {"self", {{"client", "self"}}}}}});
    EXPECT_EQ(client.GetView().state, State::Connecting);
    ASSERT_TRUE(client.Execute(Action::End, "ringing", now));
}
TEST_F(VacsClientTest, DesktopStoreWithoutOutgoingCallClearsRemoteDial) {
    Boot(); ASSERT_TRUE(client.Execute(Action::Call, "", now, "peer")); Reply(sent.back(), "outgoing");
    Event("signaling:connected", {{"client", {{"positionId", "EKCH_TWR"}}}});
    Reply(sent.back(), snapshot); Reply(sent.back()); Store({{"prio", false}, {"incomingCalls", Json::array()}});
    EXPECT_EQ(client.GetView().state, State::Idle);
}
TEST_F(VacsClientTest, AcceptsV3SnapshotWithoutLegacyCallFields) {
    ASSERT_FALSE(snapshot.contains("incomingCalls"));
    ASSERT_FALSE(snapshot.contains("outgoingCall"));
    Boot();
    EXPECT_EQ(client.GetView().state, State::Idle);
    EXPECT_TRUE(client.TakeError().empty());
    for (const auto* name : {"signaling:call-invitation", "signaling:outgoing-call", "signaling:call-update", "webrtc:call-degraded", "webrtc:call-reconnecting"}) {
        EXPECT_TRUE(std::any_of(sent.begin(), sent.end(), [&](const Json& message) { return message.value("event", "") == name; })) << name;
    }
    for (const auto* name : {"signaling:call-invite", "signaling:outgoing-call-accepted", "signaling:add-incoming-to-call-list"}) {
        EXPECT_FALSE(std::any_of(sent.begin(), sent.end(), [&](const Json& message) { return message.value("event", "") == name; })) << name;
    }
}
TEST_F(VacsClientTest, RejectsMalformedSessionSnapshotBeforeRequestingCallRecovery) {
    snapshot["clients"] = nullptr;
    client.Open(now); Reply(sent.back()); Reply(sent.back(), snapshot);
    EXPECT_EQ(client.GetView().state, State::Synchronizing);
    EXPECT_FALSE(client.TakeError().empty());
    EXPECT_EQ(sent.back()["cmd"], "remote_get_session_state");
}
TEST_F(VacsClientTest, PartialCallSyncCannotCompleteRecovery) {
    Boot(false);
    Store({{"prio", false}});
    EXPECT_EQ(client.GetView().state, State::Synchronizing);
    Store({{"prio", false}, {"incomingCalls", Json::array()}, {"callDisplay", nullptr}});
    EXPECT_EQ(client.GetView().state, State::Synchronizing);
    Store({{"prio", false}, {"incomingCalls", Json::array()}});
    EXPECT_EQ(client.GetView().state, State::Idle);
}
TEST_F(VacsClientTest, DesktopIncomingQueueRecoversAndNullLiveSyncPreservesIt) {
    Boot(false);
    Store({{"prio", false}, {"incomingCalls", Json::array({CallData("first"), CallData("second"), CallData("first")})}});
    EXPECT_EQ(client.GetView().callId, "first");
    EXPECT_EQ(client.GetView().waiting, 2);
    Store({{"prio", true}, {"callDisplay", nullptr}, {"incomingCalls", nullptr}});
    EXPECT_EQ(client.GetView().callId, "first");
    EXPECT_EQ(client.GetView().waiting, 2);
    Store({{"prio", false}, {"incomingCalls", Json::array({CallData("second")})}});
    EXPECT_EQ(client.GetView().callId, "second");
    EXPECT_EQ(client.GetView().waiting, 1);
}
TEST_F(VacsClientTest, IncomingHistoryAndEmptyRosterDoNotCreateOrEndInvitations) {
    Boot();
    Event("signaling:add-incoming-to-call-list", {{"callId", "history"}});
    EXPECT_EQ(client.GetView().state, State::Idle);
    Invite("ringing");
    Event("signaling:call-update", {{"callId", "ringing"}, {"invitedTargets", Json::array()}, {"joinedParticipants", Json::object()}});
    EXPECT_EQ(client.GetView().state, State::Incoming);
    Event("signaling:force-call-end", "ringing");
    EXPECT_EQ(client.GetView().state, State::Idle);
}
TEST_F(VacsClientTest, OutgoingEventBeforeDialReplyPreservesConfirmedConnection) {
    Boot(); ASSERT_TRUE(client.Execute(Action::Call, "", now, "peer"));
    const auto dial = sent.back();
    const Json outgoing{{"callId", "outgoing"}, {"source", dial["args"]["source"]}, {"targets", dial["args"]["targets"]}, {"prio", false}};
    Event("signaling:outgoing-call", outgoing);
    Event("signaling:call-update", {{"callId", "outgoing"}, {"invitedTargets", Json::array()}, {"joinedParticipants", {{"peer", {{"client", "peer"}}}}}});
    PeerEvent("connected", "outgoing"); Reply(dial, "outgoing");
    EXPECT_EQ(client.GetView().state, State::Connected);
    EXPECT_TRUE(client.GetView().actionable);
    EXPECT_EQ(sent.back(), dial);
    Event("signaling:outgoing-call", outgoing);
    EXPECT_EQ(client.GetView().state, State::Connected);
    Event("webrtc:call-error", {{"callId", "outgoing"}, {"origin", {{"type", "client"}, {"value", "peer"}}}, {"reason", "peer failed"}});
    EXPECT_EQ(client.GetView().state, State::Idle);
    EXPECT_TRUE(client.GetView().actionable);
}
TEST_F(VacsClientTest, RepeatedOutgoingEventSkipsJoinedTargetsAndPreservesPendingInvitations) {
    for (const auto& target : {Json{{"client", "peer"}}, Json{{"position", "EKCH_APP"}}, Json{{"station", "APP"}}}) {
        Boot();
        const Json outgoing{{"callId", "conference"}, {"targets", Json::array({target, {{"client", "other"}}})}};
        Event("signaling:outgoing-call", outgoing);
        Event("signaling:call-update", {{"callId", "conference"}, {"invitedTargets", Json::array({{{"client", "other"}}})}, {"joinedParticipants", {{"peer", target}}}});
        PeerEvent("connected", "conference");
        Event("signaling:outgoing-call", outgoing);
        Event("webrtc:call-error", {{"callId", "conference"}, {"origin", {{"type", "client"}, {"value", "peer"}}}, {"reason", "peer failed"}});
        EXPECT_EQ(client.GetView().state, State::Connecting);
        EXPECT_EQ(client.GetView().callId, "conference");
        Event("signaling:call-reject", {{"callId", "conference"}, {"targets", Json::array({{{"client", "other"}}})}});
        EXPECT_EQ(client.GetView().state, State::Idle) << target;
    }
}
TEST_F(VacsClientTest, RepeatedOutgoingClientTargetDoesNotReinviteJoinedPosition) {
    Boot();
    const Json outgoing{{"callId", "outgoing"}, {"targets", Json::array({{{"client", "peer"}}})}};
    Event("signaling:outgoing-call", outgoing);
    Event("signaling:call-update", {{"callId", "outgoing"}, {"invitedTargets", Json::array()}, {"joinedParticipants", {{"peer", {{"position", "EKCH_APP"}}}}}});
    PeerEvent("connected", "outgoing");
    Event("signaling:outgoing-call", outgoing);
    Event("webrtc:call-error", {{"callId", "outgoing"}, {"origin", {{"type", "client"}, {"value", "peer"}}}, {"reason", "peer failed"}});
    EXPECT_EQ(client.GetView().state, State::Idle);
}
TEST_F(VacsClientTest, RepeatedOutgoingEventAfterDesktopRecoverySkipsJoinedStation) {
    Boot();
    auto call = CallData("recovered");
    call["joinedParticipants"] = {{"peer", {{"target", {{"station", "APP"}}}, {"state", "connected"}}}};
    Store({{"prio", false}, {"incomingCalls", Json::array()}, {"callDisplay", {{"type", "accepted"}, {"call", call}}}});
    Event("signaling:outgoing-call", {{"callId", "recovered"}, {"targets", Json::array({{{"station", "APP"}}})}});
    Event("webrtc:call-error", {{"callId", "recovered"}, {"origin", {{"type", "client"}, {"value", "peer"}}}, {"reason", "peer failed"}});
    EXPECT_EQ(client.GetView().state, State::Idle);
}
TEST_F(VacsClientTest, DesktopRecoverySkipsFinishedDisplayButRestoresWaitingCalls) {
    Boot(); Connected();
    ASSERT_TRUE(client.Execute(Action::End, "active", now));
    now += std::chrono::seconds(10); client.Tick(now);
    Event("signaling:call-end", "active");
    Reply(sent.back(), snapshot); // The call-end event invalidates the first session snapshot.
    ASSERT_EQ(sent.back()["cmd"], "remote_get_session_state");
    Reply(sent.back(), snapshot);
    ASSERT_EQ(sent.back()["cmd"], "remote_request_store_sync");
    Reply(sent.back());
    EXPECT_EQ(client.GetView().state, State::Synchronizing);
    auto call = CallData("active");
    call["joinedParticipants"] = {{"peer", {{"target", {{"client", "peer"}}}, {"state", "connected"}}}};
    const Json display{{"type", "accepted"}, {"call", call}};
    Store({{"prio", false}, {"callDisplay", display}, {"incomingCalls", nullptr}});
    EXPECT_EQ(client.GetView().state, State::Synchronizing);
    Store({{"prio", false}, {"callDisplay", nullptr}, {"incomingCalls", Json::array({CallData("waiting")})}});
    EXPECT_EQ(client.GetView().state, State::Synchronizing);
    Store({{"prio", false}, {"callDisplay", display}, {"incomingCalls", Json::array({CallData("waiting")})}});
    EXPECT_EQ(client.GetView().state, State::Incoming);
    EXPECT_EQ(client.GetView().callId, "waiting");
    EXPECT_EQ(client.GetView().waiting, 1);
    EXPECT_TRUE(client.GetView().actionable);
}
TEST_F(VacsClientTest, LateAcceptResponseCannotReplaceNewerDesktopCall) {
    Boot(); Invite("first");
    ASSERT_TRUE(client.Execute(Action::Answer, "first", now));
    const auto accept = sent.back();
    Connected("newer");
    Reply(accept);
    EXPECT_EQ(client.GetView().state, State::Connected);
    EXPECT_EQ(client.GetView().callId, "newer");
    EXPECT_EQ(client.GetView().label, "EKCH_APP");
    ASSERT_TRUE(client.Execute(Action::End, "newer", now));
    EXPECT_EQ(sent.back()["args"]["callId"], "newer");
}
TEST_F(VacsClientTest, DesktopOutgoingEventCanBeCancelledFromPlugin) {
    Boot();
    Event("signaling:outgoing-call", {{"callId", "desktop"}, {"source", {{"clientId", "self"}}}, {"targets", Json::array({{{"client", "peer"}}})}, {"prio", false}});
    EXPECT_EQ(client.GetView().state, State::Outgoing);
    ASSERT_TRUE(client.Execute(Action::Cancel, "desktop", now));
    EXPECT_EQ(sent.back()["cmd"], "signaling_end_call");
    Reply(sent.back());
    EXPECT_EQ(client.GetView().state, State::Idle);
}
TEST_F(VacsClientTest, OutgoingCallEndsOnEmptyRosterAndCannotBeResurrected) {
    Boot(); ASSERT_TRUE(client.Execute(Action::Call, "", now, "peer"));
    const auto dial = sent.back(); Reply(dial, "outgoing");
    Event("signaling:call-end", "outgoing");
    EXPECT_EQ(client.GetView().state, State::Outgoing);
    Event("signaling:call-update", {{"callId", "outgoing"}, {"invitedTargets", Json::array()}, {"joinedParticipants", Json::object()}});
    EXPECT_EQ(client.GetView().state, State::Idle);
    Event("signaling:outgoing-call", {{"callId", "outgoing"}, {"targets", dial["args"]["targets"]}});
    PeerEvent("connected", "outgoing");
    EXPECT_EQ(client.GetView().state, State::Idle);
}
TEST_F(VacsClientTest, ConferenceTracksConnectionsPerPeerAndIgnoresOwnConnection) {
    Boot(); Connected();
    PeerEvent("connected", "active", "other");
    PeerEvent("disconnected", "active");
    EXPECT_EQ(client.GetView().state, State::Connected);
    PeerEvent("reconnecting", "active", "other");
    EXPECT_EQ(client.GetView().state, State::Connecting);
    PeerEvent("connected", "active", "self");
    EXPECT_EQ(client.GetView().state, State::Connecting);
    PeerEvent("degraded", "active");
    EXPECT_EQ(client.GetView().state, State::Connected);
    EXPECT_TRUE(client.Execute(Action::End, "active", now));
}
TEST_F(VacsClientTest, DesktopRecoveryUsesRemoteParticipantStates) {
    Boot(false);
    auto call = CallData("recovering");
    call["joinedParticipants"] = {{"self", {{"target", {{"client", "self"}}}, {"state", "connected"}}},
                                   {"peer", {{"target", {{"client", "peer"}}}, {"state", "disconnected"}}}};
    Store({{"prio", false}, {"incomingCalls", Json::array()}, {"callDisplay", {{"type", "accepted"}, {"call", call}}}});
    EXPECT_EQ(client.GetView().state, State::Connecting);
    EXPECT_NE(client.GetView().tooltip.find("Reconnecting to EKCH_APP"), std::string::npos);
    PeerEvent("degraded", "recovering");
    EXPECT_EQ(client.GetView().state, State::Connected);
}
TEST_F(VacsClientTest, ConferenceRosterReplacesDepartedPeersAndPreservesConnectionState) {
    Boot(); Connected(); PeerEvent("connected", "active", "other");
    const Json update{{"callId", "active"}, {"invitedTargets", Json::array()}, {"joinedParticipants", {{"other", {{"client", "other"}}}}}};
    Event("signaling:call-update", update); Event("signaling:call-update", update);
    EXPECT_EQ(client.GetView().state, State::Connected);
    EXPECT_EQ(client.GetView().label, "other");
    PeerEvent("disconnected", "active", "other");
    EXPECT_EQ(client.GetView().state, State::Connecting);
}
TEST_F(VacsClientTest, RejectingOneConferenceTargetDoesNotEndOtherInvitations) {
    Boot();
    Event("signaling:outgoing-call", {{"callId", "conference"}, {"targets", Json::array({{{"client", "peer"}}, {{"client", "other"}}})}});
    Event("signaling:call-reject", {{"callId", "conference"}, {"targets", Json::array({{{"client", "peer"}}})}});
    EXPECT_EQ(client.GetView().state, State::Outgoing);
    EXPECT_EQ(client.GetView().label, "other");
    Event("signaling:call-reject", {{"callId", "conference"}, {"targets", Json::array({{{"client", "other"}}})}});
    EXPECT_EQ(client.GetView().state, State::Idle);
}
TEST_F(VacsClientTest, ClientScopedErrorKeepsOtherConferencePeersConnected) {
    Boot(); Connected(); PeerEvent("connected", "active", "other");
    Event("webrtc:call-error", {{"callId", "active"}, {"origin", {{"type", "client"}, {"value", "peer"}}}, {"reason", "peer failed"}});
    EXPECT_EQ(client.GetView().state, State::Connected);
    EXPECT_EQ(client.GetView().label, "other");
    Event("webrtc:call-error", {{"callId", "active"}, {"origin", {{"type", "client"}, {"value", "other"}}}, {"reason", "call ended"}, {"callEnded", true}});
    EXPECT_EQ(client.GetView().state, State::Idle);
}
TEST(VacsUrlTest, HostOnlyNormalization) {
    EXPECT_EQ(VacsService::BuildUrl(""), "ws://localhost:9600/ws");
    EXPECT_EQ(VacsService::BuildUrl("vacs.local"), "ws://vacs.local:9600/ws");
    EXPECT_EQ(VacsService::BuildUrl("::1"), "ws://[::1]:9600/ws");
    EXPECT_EQ(VacsService::BuildUrl("[::1]"), "ws://[::1]:9600/ws");
    for (const auto* host : {"http://localhost", "ws://localhost/ws", "localhost:9600", "host/path", "host?x", "user@host"}) EXPECT_TRUE(VacsService::BuildUrl(host).empty()) << host;
}

namespace {
    struct FakeSocketState {
        FlightStrips::websocket::WebSocketStatus status = FlightStrips::websocket::WEBSOCKET_STATUS_CONNECTED;
        message_callback receive;
        on_connected_callback open;
        std::vector<Json> sent;
        std::atomic<bool> destroyed = false;
        bool autoOpen = true;
        std::chrono::milliseconds destroyDelay{0};
    };
    class FakeSocket final : public FlightStrips::websocket::WebSocket::ImplBase {
    public:
        explicit FakeSocket(std::shared_ptr<FakeSocketState> state) : state_(std::move(state)) {}
        // Stands in for ASIO joining a resolver worker blocked in getaddrinfo.
        ~FakeSocket() override { std::this_thread::sleep_for(state_->destroyDelay); state_->destroyed = true; }
        void Connect() override { if (state_->autoOpen) state_->open(); }
        void Disconnect() override { state_->status = FlightStrips::websocket::WEBSOCKET_STATUS_DISCONNECTED; }
        void Send(const std::string& raw) override { state_->sent.push_back(Json::parse(raw)); }
        FlightStrips::websocket::WebSocketStatus GetStatus() const override { return state_->status; }
    private:
        std::shared_ptr<FakeSocketState> state_;
    };
}
class VacsServiceTest : public testing::Test {
protected:
    std::string path;
    std::shared_ptr<FlightStrips::configuration::UserConfig> config;
    std::vector<std::shared_ptr<FakeSocketState>> sockets;
    std::unique_ptr<VacsService> service;
    Time now{};
    bool autoOpen = true;
    std::chrono::milliseconds destroyDelay{0};
    static bool Destroyed(const std::shared_ptr<FakeSocketState>& state) {
        const auto deadline = std::chrono::steady_clock::now() + std::chrono::seconds(5);
        while (!state->destroyed && std::chrono::steady_clock::now() < deadline) std::this_thread::sleep_for(std::chrono::milliseconds(1));
        return state->destroyed;
    }
    void SetUp() override {
        char directory[MAX_PATH]; GetTempPathA(MAX_PATH, directory);
        path = std::string(directory) + "vacs_service_" + std::to_string(GetCurrentProcessId()) + "_" + std::to_string(GetTickCount64()) + ".ini";
        config = std::make_shared<FlightStrips::configuration::UserConfig>(path);
        service = std::make_unique<VacsService>(config, [this](const std::string& url, message_callback receive, on_connected_callback open) {
            EXPECT_EQ(url, "ws://localhost:9600/ws");
            auto state = std::make_shared<FakeSocketState>(); state->receive = std::move(receive); state->open = std::move(open);
            state->autoOpen = autoOpen; state->destroyDelay = destroyDelay;
            sockets.push_back(state);
            return std::make_unique<FlightStrips::websocket::WebSocket>(std::make_unique<FakeSocket>(state));
        });
    }
    void TearDown() override { service.reset(); DeleteFileA(path.c_str()); }
};
TEST_F(VacsServiceTest, OffNeverCreatesSocketAndMiniPreferenceIsIndependent) {
    service->Tick(now); EXPECT_TRUE(sockets.empty());
    service->ToggleMini(); EXPECT_TRUE(service->MiniPreference()); EXPECT_FALSE(service->ShowMini());
    service->ToggleEnabled(); service->Tick(now); ASSERT_EQ(sockets.size(), 1);
    EXPECT_TRUE(service->ShowMini());
    service->ToggleEnabled(); EXPECT_TRUE(Destroyed(sockets[0])); EXPECT_TRUE(service->MiniPreference());
    EXPECT_TRUE(sockets[0]->sent.empty());
}
TEST_F(VacsServiceTest, SocketCallbacksAreQueuedAndScreensShareOneConnection) {
    int first = 0, second = 0;
    service->AttachScreen(&first, [&] { ++first; }); service->AttachScreen(&second, [&] { ++second; });
    service->ToggleEnabled(); service->Tick(now); ASSERT_EQ(sockets.size(), 1);
    EXPECT_TRUE(sockets[0]->sent.empty());
    service->Tick(now); EXPECT_EQ(service->GetView().state, State::Synchronizing);
    EXPECT_FALSE(sockets[0]->sent.empty());
    const auto firstCount = first, secondCount = second;
    service->ToggleMini(); EXPECT_GT(first, firstCount); EXPECT_GT(second, secondCount);
    service->DetachScreen(&second); const auto detached = second;
    service->Tick(now); EXPECT_EQ(second, detached); EXPECT_EQ(sockets.size(), 1);
}
TEST_F(VacsServiceTest, ReconnectUsesBoundedBackoffAndIgnoresObsoleteEvents) {
    service->ToggleEnabled(); service->Tick(now); service->Tick(now);
    auto old = sockets[0]; old->status = FlightStrips::websocket::WEBSOCKET_STATUS_FAILED;
    service->Tick(now); EXPECT_TRUE(Destroyed(old));
    old->open(); old->receive(R"({"type":"event","name":"auth:authenticated","payload":"old"})");
    now += std::chrono::milliseconds(999); service->Tick(now); EXPECT_EQ(sockets.size(), 1);
    now += std::chrono::milliseconds(1); service->Tick(now); ASSERT_EQ(sockets.size(), 2);
    EXPECT_TRUE(sockets[1]->sent.empty());
    EXPECT_EQ(service->GetView().state, State::Reconnecting);
    service->Tick(now); EXPECT_EQ(service->GetView().state, State::Synchronizing);
    service->Stop(); EXPECT_TRUE(Destroyed(sockets[1]));
    const auto count = sockets.size(); service->Tick(now + std::chrono::seconds(60)); EXPECT_EQ(sockets.size(), count);
}

TEST_F(VacsServiceTest, SlowSocketTeardownNeverBlocksToggleOrTimer) {
    using namespace std::chrono;
    destroyDelay = milliseconds(1200);
    service->ToggleEnabled(); service->Tick(now); service->Tick(now); ASSERT_EQ(sockets.size(), 1);
    auto started = steady_clock::now(); service->ToggleEnabled();
    EXPECT_LT(steady_clock::now() - started, milliseconds(200));
    EXPECT_FALSE(sockets[0]->destroyed);
    sockets[0]->receive(R"({"type":"event","name":"auth:authenticated","payload":"late"})");
    service->ToggleEnabled(); service->Tick(now); ASSERT_EQ(sockets.size(), 2);
    sockets[1]->status = FlightStrips::websocket::WEBSOCKET_STATUS_FAILED;
    started = steady_clock::now(); service->Tick(now);
    EXPECT_LT(steady_clock::now() - started, milliseconds(200));
    EXPECT_EQ(service->GetView().state, State::Reconnecting);
    EXPECT_TRUE(Destroyed(sockets[0])); EXPECT_TRUE(Destroyed(sockets[1]));
}
TEST_F(VacsServiceTest, TickProcessesBoundedEventBatches) {
    service->ToggleEnabled(); service->Tick(now); ASSERT_EQ(sockets.size(), 1);
    for (int i = 0; i < 100; ++i) sockets[0]->receive(Json{{"type", "event"}, {"name", "signaling:client-connected"}, {"payload", {{"id", "client" + std::to_string(i)}}}}.dump());
    ASSERT_EQ(service->QueuedEvents(), 101);
    service->Tick(now); EXPECT_EQ(service->QueuedEvents(), 37);
    service->Tick(now); EXPECT_EQ(service->QueuedEvents(), 0);
}

TEST_F(VacsServiceTest, RepeatedFailuresBackOffOneTwoFourEightSixteenThirtySeconds) {
    autoOpen = false; service->ToggleEnabled(); service->Tick(now);
    for (const int delay : {1, 2, 4, 8, 16, 30, 30}) {
        sockets.back()->status = FlightStrips::websocket::WEBSOCKET_STATUS_FAILED;
        const auto count = sockets.size(); service->Tick(now);
        EXPECT_EQ(service->GetView().label, "Retry in " + std::to_string(delay) + "s");
        now += std::chrono::seconds(delay) - std::chrono::milliseconds(1);
        service->Tick(now); EXPECT_EQ(sockets.size(), count);
        now += std::chrono::milliseconds(1); service->Tick(now); EXPECT_EQ(sockets.size(), count + 1);
    }
}

TEST_F(VacsClientTest, EndTimeoutPreservesConfirmedDisplayButGatesFurtherActions) {
    Boot(); Connected(); ASSERT_TRUE(client.Execute(Action::End, "active", now));
    now += std::chrono::seconds(10); client.Tick(now);
    EXPECT_EQ(client.GetView().state, State::Connected);
    EXPECT_EQ(client.GetView().callId, "active");
    EXPECT_FALSE(client.GetView().actionable);
    EXPECT_FALSE(client.Execute(Action::End, "active", now));
}

TEST_F(VacsServiceTest, RelaySnapshotAndNativePanelShareClientsAndPendingActions) {
    EXPECT_EQ(VacsRelay::Snapshot(*service, 1).status(), "disabled");
    service->ToggleEnabled(); service->Tick(now); service->Tick(now);
    ASSERT_EQ(sockets.size(), 1);
    const auto reply = [&](Json data) {
        const auto request = sockets[0]->sent.back();
        sockets[0]->receive(Json{{"type", "response"}, {"id", request["id"]}, {"ok", true}, {"data", data}}.dump());
        service->Tick(now);
    };
    reply(nullptr);
    reply({{"connectionState", "connected"}, {"clientId", "self"},
        {"sessionInfo", {{"client", {{"id", "self"}, {"positionId", "EKCH_TWR"}}}}},
        {"clients", Json::array({{{"id", "peer"}, {"displayName", "Approach"}, {"positionId", "EKCH_APP"}, {"frequency", "119.800"}}})},
        {"incomingCalls", Json::array()}, {"outgoingCall", nullptr}});
    reply(nullptr);
    sockets[0]->receive(Json{{"type", "event"}, {"name", "store:sync"},
        {"payload", {{"store", "call"}, {"state", {{"prio", false}, {"incomingCalls", Json::array()}}}}}}.dump());
    service->Tick(now);
    const auto state = VacsRelay::Snapshot(*service, 2);
    EXPECT_EQ(state.status(), "idle"); EXPECT_TRUE(state.actionable());
    EXPECT_EQ(state.own_client_id(), "self"); EXPECT_EQ(state.own_position_id(), "EKCH_TWR");
    ASSERT_EQ(state.clients_size(), 1); EXPECT_EQ(state.clients(0).position_id(), "EKCH_APP");
    EXPECT_TRUE(service->Execute(Action::Call, "", "peer"));
    EXPECT_EQ(sockets[0]->sent.back()["args"]["targets"], Json::array({{{"client", "peer"}}}));
    EXPECT_TRUE(VacsRelay::Snapshot(*service, 3).pending());
    EXPECT_FALSE(service->GetView().actionable);
    EXPECT_FALSE(service->Execute(Action::Call, "", "peer"));
    EXPECT_FALSE(service->Execute(Action::End, "stale"));
}
