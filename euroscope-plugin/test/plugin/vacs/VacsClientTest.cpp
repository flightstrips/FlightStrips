#include <gtest/gtest.h>
#include "vacs/VacsClient.h"
#include "vacs/VacsService.h"

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
        {"incomingCalls", Json::array()}, {"outgoingCall", nullptr}, {"defaultCallSources", Json::array()}
    };
    void Reply(const Json& request, Json data = nullptr, bool ok = true) {
        client.Receive(Json{{"type", "response"}, {"id", request["id"]}, {"ok", ok}, {"data", data}, {"error", {{"detail", "rejected"}}}}.dump(), now);
    }
    void Event(const std::string& name, Json payload) { client.Receive(Json{{"type", "event"}, {"name", name}, {"payload", payload}}.dump(), now); }
    void Store(Json state) { Event("store:sync", {{"store", "call"}, {"state", state}}); }
    void Boot(bool recover = true) {
        client.Open(now);
        Reply(sent.back());
        Reply(sent.back(), snapshot);
        Reply(sent.back());
        if (recover) Store({{"prio", false}});
    }
    void Invite(const std::string& id) { Event("signaling:call-invite", {{"callId", id}, {"source", {{"clientId", "peer"}}}}); }
    void Connected(const std::string& id = "active") {
        Store({{"prio", false}, {"callDisplay", {{"type", "accepted"}, {"call", {{"callId", id}, {"source", {{"clientId", "peer"}}}}}, {"connectionState", "connected"}}}});
    }
};

TEST_F(VacsClientTest, RequiresDesktopStoreRecoveryBeforeIdle) {
    Boot(false);
    EXPECT_EQ(client.GetView().state, State::Synchronizing);
    Store({{"prio", false}, {"callDisplay", nullptr}});
    EXPECT_EQ(client.GetView().state, State::Synchronizing);
    Store({{"prio", false}});
    EXPECT_EQ(client.GetView().state, State::Idle);
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
    EXPECT_EQ(request["cmd"], "signaling_start_call");
    EXPECT_EQ(request["args"]["target"], Json({{"client", "peer"}}));
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
    EXPECT_EQ(client.GetView().mini, "APP 2");
    ASSERT_TRUE(client.Execute(Action::Answer, "first", now));
    EXPECT_EQ(sent.back()["args"]["callId"], "first"); Reply(sent.back());
    EXPECT_EQ(client.GetView().state, State::Connecting);
    EXPECT_EQ(client.GetView().waiting, 1);
    Event("webrtc:call-connected", "first");
    EXPECT_EQ(client.GetView().state, State::Connected);
    Event("signaling:call-end", "first");
    EXPECT_EQ(client.GetView().callId, "second");
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
    Reply(sent.back(), snapshot); Reply(sent.back()); Store({{"prio", false}});
    EXPECT_EQ(client.GetView().state, State::Idle);
    EXPECT_EQ(std::count_if(sent.begin(), sent.end(), [](const Json& message) { return message.value("cmd", "") == "signaling_start_call"; }), 1);
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
    Event("signaling:accept-incoming-call", "first"); Event("webrtc:call-connected", "first"); Reply(request);
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
TEST_F(VacsClientTest, RestoresIncomingAndOutgoingSnapshot) {
    snapshot["incomingCalls"] = Json::array({{{"callId", "waiting"}, {"source", {{"clientId", "peer"}}}}});
    snapshot["outgoingCall"] = {{"callId", "ringing"}, {"target", {{"client", "peer"}}}};
    Boot(false); Store({{"prio", false}, {"callDisplay", {{"type", "outgoing"}, {"call", snapshot["outgoingCall"]}}}});
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
        bool destroyed = false, autoOpen = true;
    };
    class FakeSocket final : public FlightStrips::websocket::WebSocket::ImplBase {
    public:
        explicit FakeSocket(std::shared_ptr<FakeSocketState> state) : state_(std::move(state)) {}
        ~FakeSocket() override { state_->destroyed = true; }
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
    void SetUp() override {
        char directory[MAX_PATH]; GetTempPathA(MAX_PATH, directory);
        path = std::string(directory) + "vacs_service_" + std::to_string(GetCurrentProcessId()) + "_" + std::to_string(GetTickCount64()) + ".ini";
        config = std::make_shared<FlightStrips::configuration::UserConfig>(path);
        service = std::make_unique<VacsService>(config, [this](const std::string& url, message_callback receive, on_connected_callback open) {
            EXPECT_EQ(url, "ws://localhost:9600/ws");
            auto state = std::make_shared<FakeSocketState>(); state->receive = std::move(receive); state->open = std::move(open);
            state->autoOpen = autoOpen;
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
    service->ToggleEnabled(); EXPECT_TRUE(sockets[0]->destroyed); EXPECT_TRUE(service->MiniPreference());
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
    service->Tick(now); EXPECT_TRUE(old->destroyed);
    old->open(); old->receive(R"({"type":"event","name":"auth:authenticated","payload":"old"})");
    now += std::chrono::milliseconds(999); service->Tick(now); EXPECT_EQ(sockets.size(), 1);
    now += std::chrono::milliseconds(1); service->Tick(now); ASSERT_EQ(sockets.size(), 2);
    EXPECT_TRUE(sockets[1]->sent.empty());
    EXPECT_EQ(service->GetView().state, State::Reconnecting);
    service->Tick(now); EXPECT_EQ(service->GetView().state, State::Synchronizing);
    service->Stop(); EXPECT_TRUE(sockets[1]->destroyed);
    const auto count = sockets.size(); service->Tick(now + std::chrono::seconds(60)); EXPECT_EQ(sockets.size(), count);
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
