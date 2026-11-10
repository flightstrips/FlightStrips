#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include <thread>
#include "websocket/WebSocketService.h"
#include "version.h"
#include "websocket/Events.h"
#include "websocket/generated/proto/euroscope.pb.h"
#include "mock/MockFlightStripsPlugin.h"
#include "mock/MockAuthenticationService.h"
#include "handlers/ConnectionEventHandlers.h"
#include "handlers/MessageHandlers.h"
#include "flightplan/FlightPlanService.h"

using namespace FlightStrips;
using namespace FlightStrips::websocket;
using namespace FlightStrips::authentication;
using ::testing::_;
using ::testing::Invoke;
using ::testing::NiceMock;
using ::testing::Return;
using ::testing::ReturnRef;

static void SetTestRole(WebSocketService& socket, ClientState role) {
    protobuf::wire::Envelope control;
    auto context=socket.CaptureContext();context.set_transport_generation(1);
    *control.mutable_context()=context;control.mutable_control();socket.ProcessServerMessage(control.SerializeAsString());
    protobuf::wire::Envelope backend;
    context.set_membership_id("test-member");context.set_generation(1);context.set_session_id(42);context.set_airport("EKCH");context.set_environment("LIVE");
    *backend.mutable_context()=context;backend.mutable_backend_sync()->add_strips()->set_callsign("SAS123");socket.ProcessServerMessage(backend.SerializeAsString());
    protobuf::wire::Envelope event;*event.mutable_context()=context;
    event.mutable_session_info()->set_role(role==STATE_MASTER?"master":role==STATE_SLAVE?"slave":role==STATE_OBSERVER?"observer":"unknown");
    socket.ProcessServerMessage(event.SerializeAsString());
}

static protobuf::wire::MembershipContext AddMembership(WebSocketService& socket,int session,const std::string& airport,const std::string& role,uint64_t epoch=1) {
    auto context=socket.CaptureContext();context.set_transport_generation(1);
    protobuf::wire::Envelope welcome;*welcome.mutable_context()=context;welcome.mutable_control();socket.ProcessServerMessage(welcome.SerializeAsString());
    context.set_session_id(session);context.set_airport(airport);context.set_environment("LIVE");context.set_membership_id("member-"+airport);context.set_generation(session);context.set_master_epoch(epoch);
    protobuf::wire::Envelope event;*event.mutable_context()=context;event.mutable_session_info()->set_role(role);socket.ProcessServerMessage(event.SerializeAsString());
    return context;
}

TEST(MembershipStateTest, RetiresOnlyExactMembershipRegardlessOfMasterEpoch) {
    MembershipState state;protobuf::wire::Envelope event;
    auto* c=event.mutable_context();c->set_instance_id("instance");c->set_transport_generation(1);c->set_session_id(1);c->set_airport("AAAA");c->set_environment("LIVE");c->set_membership_id("a");c->set_generation(1);c->set_master_epoch(3);
    event.mutable_session_info()->set_role("master");ASSERT_TRUE(state.Accept(event,"instance",1,"LIVE"));
    auto other=event;other.mutable_context()->set_membership_id("b");other.mutable_context()->set_airport("BBBB");other.mutable_context()->set_session_id(2);ASSERT_TRUE(state.Accept(other,"instance",1,"LIVE"));
    auto stale=event;stale.mutable_context()->set_master_epoch(2);stale.mutable_backend_sync();EXPECT_FALSE(state.Accept(stale,"instance",1,"LIVE"));
    auto wrong=event;wrong.mutable_context()->set_environment("SWEATBOX");EXPECT_FALSE(state.Accept(wrong,"instance",1,"LIVE"));
    stale.mutable_membership_removed();EXPECT_FALSE(state.Accept(stale,"instance",1,"LIVE"));
    EXPECT_FALSE(state.members.contains("a"));EXPECT_TRUE(state.members.contains("b"));EXPECT_FALSE(state.Accept(event,"instance",1,"LIVE"));
}

// ---------------------------------------------------------------------------
// Mock WebSocket::ImplBase — controls GetStatus(), Connect(), Disconnect()
// ---------------------------------------------------------------------------

class MockWebSocketImpl : public WebSocket::ImplBase {
public:
    MOCK_METHOD(void,            Connect,    (),                      (override));
    MOCK_METHOD(void,            Disconnect, (),                      (override));
    MOCK_METHOD(void,            Send,       (const std::string&),    (override));
    MOCK_METHOD(WebSocketStatus, GetStatus,  (),                      (const, override));
};

// ---------------------------------------------------------------------------
// Test fixture — builds a WebSocketService through the protected seam
// constructor so no real WebSocket thread is started.
// ---------------------------------------------------------------------------

class WebSocketServiceOnTimerTest : public ::testing::Test {
protected:
    // Shared state that tests can modify before calling OnTimer.
    ConnectionState state{};
    MockWebSocketImpl* mockImpl{nullptr};   // non-owning, owned by WebSocket

    std::shared_ptr<NiceMock<MockAuthenticationService>>  mockAuth;
    std::shared_ptr<NiceMock<MockFlightStripsPlugin>>     mockPlugin;
    std::shared_ptr<handlers::ConnectionEventHandlers>    connHandlers;
    std::shared_ptr<handlers::MessageHandlers>            msgHandlers;

    // The service under test — subclassed only to reach the protected ctor.
    std::unique_ptr<WebSocketService> svc;

    void SetUp() override {
        mockAuth    = std::make_shared<NiceMock<MockAuthenticationService>>();
        mockPlugin  = std::make_shared<NiceMock<MockFlightStripsPlugin>>();
        connHandlers = std::make_shared<handlers::ConnectionEventHandlers>();
        msgHandlers  = std::make_shared<handlers::MessageHandlers>();

        // Default: disconnected, unauthenticated, no relevant state.
        ON_CALL(*mockPlugin, GetConnectionState()).WillByDefault(ReturnRef(state));
        ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(NONE));
        ON_CALL(*mockAuth, GetAccessToken()).WillByDefault(Return(""));

        auto implOwned = std::make_unique<NiceMock<MockWebSocketImpl>>();
        mockImpl = implOwned.get();
        ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_DISCONNECTED));

        auto ws = std::unique_ptr<WebSocket>(new WebSocket(std::move(implOwned)));

        // Use a helper subclass to call the protected constructor.
        struct Seam : WebSocketService {
            Seam(std::shared_ptr<authentication::IAuthenticationService> a,
                 std::shared_ptr<IFlightStripsPlugin> p,
                 std::shared_ptr<handlers::ConnectionEventHandlers> c,
                 std::shared_ptr<handlers::MessageHandlers> m,
                 std::unique_ptr<WebSocket> ws)
                : WebSocketService(std::move(a), std::move(p), std::move(c),
                                   std::move(m), std::move(ws), /*enabled=*/true) {}
        };
        svc = std::make_unique<Seam>(mockAuth, mockPlugin, connHandlers, msgHandlers, std::move(ws));
    }

    // Helpers to put the state into a "should connect" condition.
    void SetShouldConnect() {
        state.primary_frequency = "121.500";
        state.relevant_airport  = "EKCH";
        state.connection_type   = CONNECTION_TYPE_DIRECT;
        ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));
    }
};

// ---------------------------------------------------------------------------
// enabled=false path
// ---------------------------------------------------------------------------
TEST_F(WebSocketServiceOnTimerTest, AuthenticationWorkerDefersLatestTokenUntilSdkTimer) {
    SetShouldConnect();
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    AddMembership(*svc, 1, "AAAA", "master");
    EXPECT_CALL(*mockPlugin, GetConnectionState()).Times(0);
    EXPECT_CALL(*mockImpl, Send(_)).Times(0);
    std::thread authentication([&] { svc->OnTokenUpdate("old-token"); svc->OnTokenUpdate("new-token"); });
    authentication.join();
    ::testing::Mock::VerifyAndClearExpectations(mockPlugin.get());
    ::testing::Mock::VerifyAndClearExpectations(mockImpl);
    const auto sdkThread = std::this_thread::get_id();
    std::vector<std::string> tokens;
    EXPECT_CALL(*mockImpl, Send(_)).WillRepeatedly(Invoke([&](const std::string& bytes) {
        EXPECT_EQ(std::this_thread::get_id(), sdkThread);
        protobuf::wire::Envelope event; ASSERT_TRUE(event.ParseFromString(bytes));
        if (event.has_token()) tokens.push_back(event.token().token());
    }));
    svc->OnTimer(1);
    ASSERT_EQ(tokens.size(), 1u); EXPECT_EQ(tokens[0], "new-token");
    svc->OnTimer(2); EXPECT_EQ(tokens.size(), 1u);
}

TEST_F(WebSocketServiceOnTimerTest, HoldReplayCompletesIndependentlyForEachMembershipAndRejoin) {
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    const auto a = AddMembership(*svc, 1, "AAAA", "master");
    auto b = AddMembership(*svc, 2, "BBBB", "master");
    auto socket = std::shared_ptr<WebSocketService>(svc.get(), [](WebSocketService*) {});
    flightplan::FlightPlanService plans(socket, nullptr, nullptr, nullptr, nullptr);
    plans.CacheTrackedHold("SAS124", {}); plans.CacheTrackedHold("SAS125", {});
    auto* assigned = plans.GetSourceFlightPlan("SAS124");
    auto* cancelled = plans.GetSourceFlightPlan("SAS125");
    flightplan::ApplyTopSkyHoldCommand(*assigned, {flightplan::TopSkyHoldCommandType::Assign, "OLPIB", "1422"});
    flightplan::ApplyTopSkyHoldCommand(*cancelled, {flightplan::TopSkyHoldCommandType::Cancel, {}});
    assigned->hold_command_pending = cancelled->hold_command_pending = true;
    // One global source collection must not resurrect the stale annotation.
    plans.CacheTrackedHold("SAS124", {true, false, "ROSBI"});
    plans.CacheTrackedHold("SAS125", {true, false, "OLPIB"});
    Strip assignedStrip{"SAS124","BBBB","ZZZZ","","DCT","","04L","7000","7000","SID","STAR",false,"",0,0,0,"A320","M","",Position{0,0,1000},"","V","","1200","1300","","J"};
    auto cancelledStrip = assignedStrip; cancelledStrip.callsign = "SAS125";
    const SyncEvent snapshot{{assignedStrip, cancelledStrip}, {}, {}, {}};
    std::vector<protobuf::wire::Envelope> sent;
    EXPECT_CALL(*mockImpl, Send(_)).WillRepeatedly(Invoke([&](const std::string& bytes) {
        protobuf::wire::Envelope event; ASSERT_TRUE(event.ParseFromString(bytes)); sent.push_back(event);
    }));
    const auto replay = [&](const std::string& id) {
        svc->ForEachMembership([&](const std::string&) {
            if (svc->CurrentMembershipId() != id) return;
            svc->SendEvent(snapshot);
            plans.ReplayPendingHoldCommands();
        });
    };
    replay(a.membership_id());
    ASSERT_EQ(sent.size(), 1u); EXPECT_EQ(sent[0].sync().strips_size(), 0);
    EXPECT_TRUE(assigned->hold_command_pending); EXPECT_TRUE(cancelled->hold_command_pending);
    replay(b.membership_id());
    ASSERT_EQ(sent.size(), 4u); EXPECT_EQ(sent[1].sync().strips_size(), 2);
    for (size_t i = 2; i < 4; ++i) {
        EXPECT_EQ(sent[i].context().membership_id(), b.membership_id()); ASSERT_TRUE(sent[i].has_hold());
        if (sent[i].hold().callsign() == "SAS124") {
            EXPECT_EQ(sent[i].hold().hold(), "OLPIB"); EXPECT_EQ(sent[i].hold().hold_eat(), "1422");
        } else { EXPECT_EQ(sent[i].hold().callsign(), "SAS125"); EXPECT_TRUE(sent[i].hold().hold().empty()); }
    }
    EXPECT_FALSE(assigned->hold_command_pending); EXPECT_FALSE(cancelled->hold_command_pending);
    protobuf::wire::Envelope event; *event.mutable_context() = b; event.mutable_membership_removed();
    svc->ProcessServerMessage(event.SerializeAsString());
    b.set_membership_id("rejoined-BBBB"); b.set_generation(3);
    event.Clear(); *event.mutable_context() = b; event.mutable_session_info()->set_role("master");
    svc->ProcessServerMessage(event.SerializeAsString());
    replay(b.membership_id());
    ASSERT_EQ(sent.size(), 7u);
    for (size_t i = 5; i < 7; ++i) { EXPECT_TRUE(sent[i].has_hold()); EXPECT_EQ(sent[i].context().membership_id(), b.membership_id()); }
}

TEST_F(WebSocketServiceOnTimerTest, EatUsesPhysicalHoldCommandsAcrossIndependentMembershipCaches) {
    state.relevant_airport = "AAAA";
    AddMembership(*svc, 1, "AAAA", "master");
    AddMembership(*svc, 2, "BBBB", "master");
    auto socket = std::shared_ptr<WebSocketService>(svc.get(), [](WebSocketService*) {});
    flightplan::FlightPlanService plans(socket, nullptr, nullptr, nullptr, nullptr);
    svc->ForEachMembership([&](const std::string& airport) {
        plans.ApplyBackendSyncHold("SAS123", airport == "AAAA" ? "ROSBI" : "OLPIB", "enroute", "1400");
        // Backend state alone cannot establish a locally observed TopSky hold.
        EXPECT_FALSE(plans.ResolveHoldForEat("SAS123", {}).active);
    });
    plans.CacheTrackedHold("SAS123", {});
    auto* source = plans.GetSourceFlightPlan("SAS123");
    ASSERT_NE(source, nullptr);
    ASSERT_TRUE(flightplan::ApplyTopSkyHoldCommand(*source,
        {flightplan::TopSkyHoldCommandType::Assign, "OLPIB"}));
    svc->ForEachMembership([&](const std::string& airport) {
        ASSERT_NE(plans.GetFlightPlan("SAS123"), source);
        EXPECT_EQ(plans.GetFlightPlan("SAS123")->hold, airport == "AAAA" ? "ROSBI" : "OLPIB");
        EXPECT_EQ(flightplan::BuildTopSkyHoldEatCommand(plans.ResolveHoldForEat("SAS123", {}),
            "OLPIB", "enroute", "1422"), "/HOLD_EAT/1422/");
        EXPECT_TRUE(flightplan::BuildTopSkyHoldEatCommand(plans.ResolveHoldForEat("SAS123", {}),
            "ROSBI", "enroute", "1422").empty());
    });
    ASSERT_TRUE(flightplan::ApplyTopSkyHoldCommand(*source,
        {flightplan::TopSkyHoldCommandType::Cancel, {}}));
    svc->ForEachMembership([&](const std::string&) {
        // XHOLD also wins over an annotation and a backend replay that are stale.
        EXPECT_FALSE(plans.ResolveHoldForEat("SAS123", {true, false, "OLPIB"}).active);
    });
    EXPECT_EQ(plans.ResolveHoldForEat("UNKNOWN", {true, false, "ROSBI"}).point, "ROSBI");
}

TEST_F(WebSocketServiceOnTimerTest, CachedHoldReplayCannotConsumeOrSuppressTypedCommandResults) {
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    const auto a = AddMembership(*svc, 1, "AAAA", "master");
    const auto b = AddMembership(*svc, 2, "BBBB", "master");
    Strip strip{"SAS123","AAAA","BBBB","","DCT","","04L","7000","7000","SID","STAR",false,"",0,0,0,"A320","M","",Position{55,12,1000},"","V","","1200","1300","","J"};
    svc->SendEvent(SyncEvent{{strip}, {}, {}, {}});
    auto socket = std::shared_ptr<WebSocketService>(svc.get(), [](WebSocketService*) {});
    flightplan::FlightPlanService plans(socket, nullptr, nullptr, nullptr, nullptr);
    plans.CacheTrackedHold("SAS123", {true, false, "OLPIB"}, "1422");
    const HoldEvent hold("SAS123", "OLPIB", "enroute", "1422");
    const auto issue = [&](const protobuf::wire::MembershipContext& context, const std::string& id) {
        protobuf::wire::Envelope command;
        ASSERT_TRUE(command.ParseFromString(protobuf::Serialize(hold)));
        *command.mutable_context() = context;
        command.set_command_origin(protobuf::wire::COMMAND_ORIGIN_HUMAN); command.set_execution_id(id);
        svc->ProcessServerMessage(command.SerializeAsString());
    };
    std::vector<protobuf::wire::Envelope> sent;
    EXPECT_CALL(*mockImpl, Send(_)).WillRepeatedly(Invoke([&](const std::string& bytes) {
        protobuf::wire::Envelope event; ASSERT_TRUE(event.ParseFromString(bytes)); sent.push_back(event);
    }));
    const auto replayB = [&] {
        svc->ForEachMembership([&](const std::string&) {
            if (svc->CurrentMembershipId() == b.membership_id()) plans.ReplayPendingHoldCommands();
        });
    };
    const auto ordinaryB = [&](size_t index) {
        ASSERT_LT(index, sent.size()); ASSERT_TRUE(sent[index].has_hold());
        EXPECT_EQ(sent[index].context().membership_id(), b.membership_id());
        EXPECT_EQ(sent[index].command_origin(), protobuf::wire::COMMAND_ORIGIN_UNSPECIFIED);
        EXPECT_TRUE(sent[index].execution_id().empty());
    };
    issue(a, "hold-a");
    replayB();
    ASSERT_EQ(sent.size(), 1u); ordinaryB(0);
    // The replay did not consume A's ticket; the real SDK result can still complete it.
    EXPECT_TRUE(svc->SendCommandResult(hold));
    ASSERT_EQ(sent.size(), 3u); EXPECT_EQ(sent[1].execution_id(), "hold-a"); ordinaryB(2);
    issue(a, "hold-a2"); issue(b, "hold-b2");
    replayB();
    // Identical outstanding setters must not suppress the independent source replay.
    ASSERT_EQ(sent.size(), 4u); ordinaryB(3);
    protobuf::wire::Envelope removed; *removed.mutable_context() = a; removed.mutable_membership_removed();
    svc->ProcessServerMessage(removed.SerializeAsString());
    EXPECT_TRUE(svc->SendCommandResult(hold));
    ASSERT_EQ(sent.size(), 5u); EXPECT_EQ(sent[4].execution_id(), "hold-b2");
    EXPECT_EQ(sent[4].context().membership_id(), b.membership_id());
}

TEST_F(WebSocketServiceOnTimerTest, MultipleMastersAndNonMasterRouteIndependently) {
    ON_CALL(*mockImpl,GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    auto a=AddMembership(*svc,1,"AAAA","master");auto b=AddMembership(*svc,2,"BBBB","master");auto c=AddMembership(*svc,3,"CCCC","slave");
    svc->RememberAircraft("SAS123","CCCC","AAAA",55,12);
    std::vector<protobuf::wire::Envelope> sent;
    EXPECT_CALL(*mockImpl,Send(_)).WillRepeatedly(Invoke([&](const std::string& bytes){protobuf::wire::Envelope e;ASSERT_TRUE(e.ParseFromString(bytes));sent.push_back(e);}));
    svc->SendEvent(AMANRouteFactEvent("SAS123",std::optional<std::string>{"OLPIB"},"2026-11-04T12:00:00Z"));
    ASSERT_EQ(sent.size(),1u);EXPECT_EQ(sent[0].context().session_id(),3); // newly observed slave aircraft
    EXPECT_EQ(svc->Memberships().members.at(a.membership_id()).role,STATE_MASTER);
    EXPECT_EQ(svc->Memberships().members.at(b.membership_id()).role,STATE_MASTER);
    protobuf::wire::Envelope removed;*removed.mutable_context()=b;removed.mutable_membership_removed();svc->ProcessServerMessage(removed.SerializeAsString());
    EXPECT_EQ(svc->Memberships().members.size(),2u);EXPECT_TRUE(svc->ShouldSend());
}

TEST_F(WebSocketServiceOnTimerTest, RadarOnlyMovementIntroducesEachMasterBeforeItsFirstPosition) {
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    const auto a = AddMembership(*svc, 1, "AAAA", "master");
    const auto b = AddMembership(*svc, 2, "BBBB", "master");
    const auto slave = AddMembership(*svc, 3, "CCCC", "slave");
    const auto observer = AddMembership(*svc, 4, "DDDD", "observer");
    const auto geography = [&](const protobuf::wire::MembershipContext& context, double longitude) {
        protobuf::wire::Envelope event; *event.mutable_context() = context;
        event.mutable_backend_sync()->set_latitude(0);
        event.mutable_backend_sync()->set_longitude(longitude);
        svc->ProcessServerMessage(event.SerializeAsString());
    };
    geography(a, 0); geography(b, 2); geography(slave, 2); geography(observer, 2);
    StripUpdateEvent introduction{
        "VFR123", "", "", "", "", "", "", "7000", "", "", "",
        false, "", 0, 0, 0, "", "", "", Position{0, 0, 1000}, "",
        "", "", "", "", "", "", false
    };
    std::vector<protobuf::wire::Envelope> sent;
    EXPECT_CALL(*mockImpl, Send(_)).WillRepeatedly(Invoke([&](const std::string& bytes) {
        protobuf::wire::Envelope event; ASSERT_TRUE(event.ParseFromString(bytes)); sent.push_back(event);
    }));
    svc->RememberAircraft("VFR123", "", "", 0, 0, false);
    svc->SendRadarOnlyIntroduction(introduction);
    ASSERT_EQ(sent.size(), 1u);
    ASSERT_TRUE(sent[0].has_strip_update());
    EXPECT_EQ(sent[0].context().membership_id(), a.membership_id());
    EXPECT_FALSE(sent[0].strip_update().strip().has_fp());
    svc->SendRadarOnlyIntroduction(introduction);
    EXPECT_EQ(sent.size(), 1u); // Already known at A; no repeated strip update.

    // The same physical target is now inside B's range, without a flight-plan
    // callback, a full sync, or a new entry in FlightPlanService's source cache.
    introduction.position.lon = 2;
    svc->RememberAircraft("VFR123", "", "", 0, 2, false);
    EXPECT_FALSE(svc->Memberships().members.at(b.membership_id()).inventory.contains("VFR123"));
    svc->SendRadarOnlyIntroduction(introduction);
    ASSERT_EQ(sent.size(), 2u);
    ASSERT_TRUE(sent[1].has_strip_update());
    EXPECT_EQ(sent[1].context().membership_id(), b.membership_id());
    EXPECT_EQ(sent[1].strip_update().strip().position().lon(), 2);
    EXPECT_TRUE(sent[1].execution_id().empty());
    EXPECT_EQ(sent[1].command_origin(), protobuf::wire::COMMAND_ORIGIN_UNSPECIFIED);
    const auto contexts = svc->CaptureObservationContexts("VFR123");
    ASSERT_EQ(contexts.size(), 2u);
    for (const auto& context : contexts) svc->SendObservation(PositionEvent("VFR123", 0, 2, 1000), context);
    ASSERT_EQ(sent.size(), 4u);
    bool bPosition = false;
    for (size_t i = 2; i < sent.size(); ++i) {
        EXPECT_TRUE(sent[i].has_aircraft_position_update());
        if (sent[i].context().membership_id() == b.membership_id()) bPosition = true;
        EXPECT_NE(sent[i].context().membership_id(), slave.membership_id());
        EXPECT_NE(sent[i].context().membership_id(), observer.membership_id());
    }
    EXPECT_TRUE(bPosition);
    svc->SendRadarOnlyIntroduction(introduction);
    EXPECT_EQ(sent.size(), 4u);
}

TEST_F(WebSocketServiceOnTimerTest, CapturedObservationCannotSurviveElectionButControllerResultCan) {
    ON_CALL(*mockImpl,GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    auto context=AddMembership(*svc,1,"AAAA","slave");svc->RememberAircraft("SAS123","AAAA","BBBB",55,12);
    context=AddMembership(*svc,1,"AAAA","master");
    const auto observations=svc->CaptureObservationContexts("SAS123");ASSERT_EQ(observations.size(),1u);
    AddMembership(*svc,1,"AAAA","slave",2);AddMembership(*svc,1,"AAAA","master",3);
    EXPECT_CALL(*mockImpl,Send(_)).Times(1);
    svc->SendObservation(PositionEvent("SAS123",55,12,5000),observations[0]);
    svc->SendCaptured(FrequencyMessageResultEvent{},context);
    protobuf::wire::Envelope removed;*removed.mutable_context()=context;removed.mutable_membership_removed();svc->ProcessServerMessage(removed.SerializeAsString());
    svc->SendCaptured(FrequencyMessageResultEvent{},context);
}

TEST_F(WebSocketServiceOnTimerTest, SharedFullSnapshotBuildsIndependentInventoriesAndProjections) {
    ON_CALL(*mockImpl,GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    const auto a=AddMembership(*svc,1,"AAAA","master");
    Strip strip{"SAS123","AAAA","BBBB","","DCT","","04L","7000","7000","SID","STAR",false,"",0,0,0,"A320","M","",Position{55,12,1000},"","V","","1200","1300","","J"};
    auto bOnly = strip; bOnly.callsign = "SAS456"; bOnly.origin = "BBBB"; bOnly.destination = "ZZZZ";
    const SyncEvent physicalSnapshot{{strip, bOnly},{},{},{}};
    svc->RememberRunways("SAS123","04L","22R");
    std::vector<protobuf::wire::Envelope> sent;
    EXPECT_CALL(*mockImpl,Send(_)).Times(2).WillRepeatedly(Invoke([&](const std::string& bytes){protobuf::wire::Envelope e;ASSERT_TRUE(e.ParseFromString(bytes));sent.push_back(e);}));
    svc->SendEvent(physicalSnapshot);
    // Reuse the physical collection when B joins later in the dispatch batch.
    const auto b=AddMembership(*svc,2,"BBBB","master");
    AddMembership(*svc,3,"CCCC","slave");
    svc->ForEachMembership([&](const std::string& airport) {
        if (airport == "BBBB") svc->SendEvent(physicalSnapshot);
    });
    ASSERT_EQ(sent.size(),2u);
    for(const auto& e:sent) {
        if(e.context().airport()=="AAAA") {
            ASSERT_EQ(e.sync().strips_size(),1);
            EXPECT_EQ(e.sync().strips(0).runway(),"04L");EXPECT_TRUE(e.sync().strips(0).star().empty());
        } else {
            EXPECT_EQ(e.context().airport(),"BBBB");ASSERT_EQ(e.sync().strips_size(),2);
            EXPECT_EQ(e.sync().strips(0).runway(),"22R");EXPECT_EQ(e.sync().strips(0).star(),"STAR");EXPECT_EQ(e.sync().strips(0).eldt(),"1300");
            EXPECT_EQ(e.sync().strips(1).callsign(),"SAS456");
        }
    }
    EXPECT_TRUE(svc->Memberships().members.at(a.membership_id()).inventory.contains("SAS123"));
    EXPECT_TRUE(svc->Memberships().members.at(b.membership_id()).inventory.contains("SAS123"));
    EXPECT_FALSE(svc->Memberships().members.at(a.membership_id()).inventory.contains("SAS456"));
    EXPECT_TRUE(svc->Memberships().members.at(b.membership_id()).inventory.contains("SAS456"));
    AddMembership(*svc,1,"AAAA","slave",2);
    EXPECT_TRUE(svc->Memberships().members.at(b.membership_id()).inventory.contains("SAS123"));
    EXPECT_EQ(svc->Memberships().members.at(b.membership_id()).role,STATE_MASTER);
}



TEST_F(WebSocketServiceOnTimerTest, IncrementalArrivalFieldsAreProjectedForTheDestinationMembership) {
    state.relevant_airport = "AAAA";
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    AddMembership(*svc, 1, "AAAA", "master");
    AddMembership(*svc, 2, "BBBB", "master");
    Strip strip{"SAS123","AAAA","BBBB","","DCT","","04L","7000","7000","SID","STAR",false,"",0,0,0,"A320","M","",Position{55,12,1000},"","V","","1200","1300","","J"};
    svc->RememberRunways("SAS123", "04L", "22R");
    svc->SendEvent(SyncEvent{{strip},{},{},{}});
    std::vector<protobuf::wire::Envelope> sent;
    EXPECT_CALL(*mockImpl, Send(_)).Times(2).WillRepeatedly(Invoke([&](const std::string& bytes) {
        protobuf::wire::Envelope event; ASSERT_TRUE(event.ParseFromString(bytes)); sent.push_back(event);
    }));
    svc->SendEvent(StripUpdateEvent{"SAS123","AAAA","BBBB","","DCT","","04L","7000","7000","SID","NEWSTAR",false,"",0,0,0,"A320","M","",Position{55,12,1000},"","V","","1200","1330","","J"});
    ASSERT_EQ(sent.size(), 2u);
    for (const auto& event : sent) {
        ASSERT_TRUE(event.has_strip_update());
        const auto& updated = event.strip_update().strip();
        if (event.context().airport() == "AAAA") {
            EXPECT_EQ(updated.runway(), "04L");
            EXPECT_TRUE(updated.star().empty());
            EXPECT_TRUE(updated.eldt().empty());
        } else {
            EXPECT_EQ(event.context().airport(), "BBBB");
            EXPECT_EQ(updated.runway(), "22R");
            EXPECT_EQ(updated.star(), "NEWSTAR");
            EXPECT_EQ(updated.eldt(), "1330");
        }
    }
}

TEST_F(WebSocketServiceOnTimerTest, SlaveReplaysActiveAnnotationAndAuthoritativeCancellation) {
    auto socket = std::shared_ptr<WebSocketService>(std::move(svc));
    flightplan::FlightPlanService plans(socket, {}, {}, {}, nullptr);
    plans.SetStand("SAS123", "");
    auto* plan = plans.GetFlightPlan("SAS123");
    const flightplan::TopSkyHold active{true, false, "OLPIB"};
    flightplan::ApplyHold(*plan, active, "1422");
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    SetTestRole(*socket,STATE_SLAVE);
    std::vector<protobuf::wire::HoldEvent> reports;
    EXPECT_CALL(*mockImpl, Send(_)).Times(3).WillRepeatedly(Invoke([&](const std::string& bytes) {
        protobuf::wire::Envelope envelope;
        ASSERT_TRUE(envelope.ParseFromString(bytes));
        ASSERT_TRUE(envelope.has_hold());
        reports.push_back(envelope.hold());
    }));
    plans.ReplayTrackedHold("SAS123", true, active, "");
    // Confirmation after an earlier rejection must replay the unchanged value.
    plans.ReplayTrackedHold("SAS123", true, active, "");
    ASSERT_TRUE(flightplan::ApplyTopSkyHoldCommand(
        *plan, {flightplan::TopSkyHoldCommandType::Cancel, {}}));
    // The stale annotation must not resurrect the hold after an XHOLD observed
    // while the backend was unavailable.
    plans.ReplayTrackedHold("SAS123", true, active, "");
    plans.SetStand("SAS124", "");
    // Without an observed command, a missing annotation is still inconclusive.
    plans.ReplayTrackedHold("SAS124", true, {}, "");
    ASSERT_EQ(reports.size(), 3u);
    EXPECT_EQ(reports[0].hold(), "OLPIB");
    EXPECT_EQ(reports[0].hold_eat(), "1422");
    EXPECT_EQ(reports[1].hold(), "OLPIB");
    EXPECT_TRUE(reports[2].hold().empty());
    EXPECT_TRUE(reports[2].hold_type().empty());
    EXPECT_TRUE(reports[2].hold_eat().empty());
    // Neither loss of tracking nor observer/disconnected status permits replay.
    plans.ReplayTrackedHold("SAS123", false, active, "");
    SetTestRole(*socket,STATE_OBSERVER);
    plans.ReplayTrackedHold("SAS123", true, active, "");
    SetTestRole(*socket,STATE_SLAVE);
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_DISCONNECTED));
    plans.ReplayTrackedHold("SAS123", true, active, "");
}

TEST_F(WebSocketServiceOnTimerTest, MasterReplaysPendingCommandWithoutTrackingOwnership) {
    auto socket = std::shared_ptr<WebSocketService>(std::move(svc));
    flightplan::FlightPlanService plans(socket, {}, {}, {}, nullptr);
    plans.SetStand("SAS125", "");
    auto* plan = plans.GetFlightPlan("SAS125");
    ASSERT_NE(plan, nullptr);
    ASSERT_TRUE(flightplan::ApplyTopSkyHoldCommand(
        *plan, {flightplan::TopSkyHoldCommandType::Assign, "OLPIB"}));
    plan->hold_command_pending = true;

    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    socket->RememberAircraft("SAS125","EKCH","EGLL",55,12);
    SetTestRole(*socket,STATE_MASTER);
    EXPECT_CALL(*mockImpl, Send(_)).WillOnce(Invoke([](const std::string& bytes) {
        protobuf::wire::Envelope envelope;
        ASSERT_TRUE(envelope.ParseFromString(bytes));
        ASSERT_TRUE(envelope.has_hold());
        EXPECT_EQ(envelope.hold().callsign(), "SAS125");
        EXPECT_EQ(envelope.hold().hold(), "OLPIB");
    }));

    plans.ReplayPendingHoldCommands();
    EXPECT_FALSE(plan->hold_command_pending);
}

TEST(WebSocketServiceDisabledTest, OnTimer_WhenDisabled_DoesNothing) {
    // Build a service with enabled=false; GetConnectionState must never be called.
    auto mockAuth   = std::make_shared<NiceMock<MockAuthenticationService>>();
    auto mockPlugin = std::make_shared<NiceMock<MockFlightStripsPlugin>>();
    auto connH      = std::make_shared<handlers::ConnectionEventHandlers>();
    auto msgH       = std::make_shared<handlers::MessageHandlers>();

    auto implOwned  = std::make_unique<NiceMock<MockWebSocketImpl>>();
    auto* impl      = implOwned.get();
    ON_CALL(*impl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_DISCONNECTED));

    struct Seam : WebSocketService {
        Seam(std::shared_ptr<authentication::IAuthenticationService> a,
             std::shared_ptr<IFlightStripsPlugin> p,
             std::shared_ptr<handlers::ConnectionEventHandlers> c,
             std::shared_ptr<handlers::MessageHandlers> m,
             std::unique_ptr<WebSocket> ws)
            : WebSocketService(std::move(a), std::move(p), std::move(c),
                               std::move(m), std::move(ws), /*enabled=*/false) {}
    };

    ConnectionState st{};
    EXPECT_CALL(*mockPlugin, GetConnectionState()).Times(0);

    auto ws  = std::unique_ptr<WebSocket>(new WebSocket(std::move(implOwned)));
    auto svc = std::make_unique<Seam>(mockAuth, mockPlugin, connH, msgH, std::move(ws));
    svc->OnTimer(1);
}

// ---------------------------------------------------------------------------
// "should_connect" gate — each condition independently blocks connection
// ---------------------------------------------------------------------------

TEST_F(WebSocketServiceOnTimerTest, OnTimer_EmptyFrequency_SchedulesControlConnection) {
    state.primary_frequency = "";
    state.relevant_airport  = "EKCH";
    state.connection_type   = CONNECTION_TYPE_DIRECT;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));

    EXPECT_CALL(*mockImpl, Connect()).Times(0);
    svc->OnTimer(1);
    EXPECT_TRUE(svc->IsPendingConnect());
}

TEST_F(WebSocketServiceOnTimerTest, OnTimer_SpuriousFrequency199_998_SchedulesControlConnection) {
    state.primary_frequency = "199.998";
    state.relevant_airport  = "EKCH";
    state.connection_type   = CONNECTION_TYPE_DIRECT;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));

    EXPECT_CALL(*mockImpl, Connect()).Times(0);
    svc->OnTimer(1);
    EXPECT_TRUE(svc->IsPendingConnect());
}

TEST_F(WebSocketServiceOnTimerTest, OnTimer_EmptyAirport_SchedulesControlConnection) {
    state.primary_frequency = "121.500";
    state.relevant_airport  = "";
    state.connection_type   = CONNECTION_TYPE_DIRECT;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));

    EXPECT_CALL(*mockImpl, Connect()).Times(0);
    svc->OnTimer(1);
    EXPECT_TRUE(svc->IsPendingConnect());
}

TEST_F(WebSocketServiceOnTimerTest, OnTimer_NoConnectionType_DoesNotScheduleConnect) {
    state.primary_frequency = "121.500";
    state.relevant_airport  = "EKCH";
    state.connection_type   = CONNECTION_TYPE_NO;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));

    EXPECT_CALL(*mockImpl, Connect()).Times(0);
    svc->OnTimer(1);
    EXPECT_FALSE(svc->IsPendingConnect());
}

TEST_F(WebSocketServiceOnTimerTest, OnTimer_NotAuthenticated_DoesNotScheduleConnect) {
    state.primary_frequency = "121.500";
    state.relevant_airport  = "EKCH";
    state.connection_type   = CONNECTION_TYPE_DIRECT;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(NONE));

    EXPECT_CALL(*mockImpl, Connect()).Times(0);
    svc->OnTimer(1);
    EXPECT_FALSE(svc->IsPendingConnect());
}

TEST_F(WebSocketServiceOnTimerTest, OnTimer_ObserverWithoutPrimary_SchedulesControlConnection) {
    state.primary_frequency = "";
    state.relevant_airport  = "EKCH";
    state.connection_type   = CONNECTION_TYPE_DIRECT;
    state.observer          = true;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));

    EXPECT_CALL(*mockImpl, Connect()).Times(0);
    svc->OnTimer(1);
    EXPECT_TRUE(svc->IsPendingConnect());
}

TEST_F(WebSocketServiceOnTimerTest, OnTimer_ObserverSpuriousFrequency199_998_SchedulesControlConnection) {
    state.primary_frequency = "199.998";
    state.relevant_airport  = "EKCH";
    state.connection_type   = CONNECTION_TYPE_DIRECT;
    state.observer          = true;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));

    EXPECT_CALL(*mockImpl, Connect()).Times(0);
    svc->OnTimer(1);
    EXPECT_TRUE(svc->IsPendingConnect());
}

// ---------------------------------------------------------------------------
// First online transition when all conditions are met — schedules the 30 s delay
// ---------------------------------------------------------------------------

TEST_F(WebSocketServiceOnTimerTest, OnTimer_AllConditionsMet_FirstTick_SetsPendingConnect) {
    SetShouldConnect();
    EXPECT_CALL(*mockImpl, Connect()).Times(0);  // not yet — delay pending
    svc->OnTimer(1);
    EXPECT_TRUE(svc->IsPendingConnect());
}

TEST_F(WebSocketServiceOnTimerTest, OnTimer_AllConditionsMet_FirstTick_DelaySecondsIsSet) {
    SetShouldConnect();
    svc->OnTimer(1);
    const auto delay = svc->GetDelaySecondsRemaining();
    EXPECT_TRUE(delay.has_value());
    EXPECT_GE(*delay, 29);
    EXPECT_LE(*delay, 30);
}

// ---------------------------------------------------------------------------
// REFRESH state also counts as authenticated
// ---------------------------------------------------------------------------

TEST_F(WebSocketServiceOnTimerTest, OnTimer_RefreshState_SetsPendingConnect) {
    state.primary_frequency = "121.500";
    state.relevant_airport  = "EKCH";
    state.connection_type   = CONNECTION_TYPE_SWEATBOX;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(REFRESH));

    svc->OnTimer(1);
    EXPECT_TRUE(svc->IsPendingConnect());
}

// ---------------------------------------------------------------------------
// Connection type variants accepted
// ---------------------------------------------------------------------------

TEST_F(WebSocketServiceOnTimerTest, OnTimer_SweatboxType_SetsPendingConnect) {
    state.primary_frequency = "121.500";
    state.relevant_airport  = "EKCH";
    state.connection_type   = CONNECTION_TYPE_SWEATBOX;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));
    svc->OnTimer(1);
    EXPECT_TRUE(svc->IsPendingConnect());
}

TEST_F(WebSocketServiceOnTimerTest, OnTimer_PlaybackType_SetsPendingConnect) {
    state.primary_frequency = "121.500";
    state.relevant_airport  = "EKCH";
    state.connection_type   = CONNECTION_TYPE_PLAYBACK;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));
    svc->OnTimer(1);
    EXPECT_TRUE(svc->IsPendingConnect());
}

TEST(WebSocketServiceStartupOnlineTest, OnTimer_WhenLoadedWhileOnline_UsesFastDelay) {
    ConnectionState state{};
    state.primary_frequency = "121.500";
    state.relevant_airport = "EKCH";
    state.connection_type = CONNECTION_TYPE_DIRECT;

    auto mockAuth = std::make_shared<NiceMock<MockAuthenticationService>>();
    auto mockPlugin = std::make_shared<NiceMock<MockFlightStripsPlugin>>();
    auto connHandlers = std::make_shared<handlers::ConnectionEventHandlers>();
    auto msgHandlers = std::make_shared<handlers::MessageHandlers>();

    ON_CALL(*mockPlugin, GetConnectionState()).WillByDefault(ReturnRef(state));
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));
    ON_CALL(*mockAuth, GetAccessToken()).WillByDefault(Return(""));

    auto implOwned = std::make_unique<NiceMock<MockWebSocketImpl>>();
    auto* mockImpl = implOwned.get();
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_DISCONNECTED));

    struct Seam : WebSocketService {
        Seam(std::shared_ptr<authentication::IAuthenticationService> a,
             std::shared_ptr<IFlightStripsPlugin> p,
             std::shared_ptr<handlers::ConnectionEventHandlers> c,
             std::shared_ptr<handlers::MessageHandlers> m,
             std::unique_ptr<WebSocket> ws)
            : WebSocketService(std::move(a), std::move(p), std::move(c),
                               std::move(m), std::move(ws), /*enabled=*/true) {}
    };

    auto ws = std::unique_ptr<WebSocket>(new WebSocket(std::move(implOwned)));
    auto svc = std::make_unique<Seam>(mockAuth, mockPlugin, connHandlers, msgHandlers, std::move(ws));

    EXPECT_CALL(*mockImpl, Connect()).Times(0);
    svc->OnTimer(1);

    const auto delay = svc->GetDelaySecondsRemaining();
    ASSERT_TRUE(delay.has_value());
    EXPECT_GE(*delay, 4);
    EXPECT_LE(*delay, 5);
}

// ---------------------------------------------------------------------------
// Disconnects when connected but conditions no longer met
// ---------------------------------------------------------------------------

TEST_F(WebSocketServiceOnTimerTest, OnTimer_AirportHintLost_RetainsPhysicalTransport) {
    // Start "connected".
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    // Conditions not met (no airport).
    state.primary_frequency = "121.500";
    state.relevant_airport  = "";
    state.connection_type   = CONNECTION_TYPE_DIRECT;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));

    EXPECT_CALL(*mockImpl, Disconnect()).Times(0);
    svc->OnTimer(1);
}

TEST_F(WebSocketServiceOnTimerTest, OnTimer_AirportHintLost_WaitsForScopedBackendRemoval) {
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    state.primary_frequency = "121.500";
    state.relevant_airport  = "";
    state.connection_type   = CONNECTION_TYPE_DIRECT;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));

    SetTestRole(*svc,STATE_MASTER);
    svc->OnTimer(1);
    // Control evidence does not locally revoke unrelated memberships.
    EXPECT_TRUE(svc->ShouldSend());
}

TEST_F(WebSocketServiceOnTimerTest, FinalMembershipRemovalKeepsControlAndFutureJoinAvailable) {
    ON_CALL(*mockPlugin, GetEligibleAirports()).WillByDefault(Return(std::vector<std::string>{"AAAA", "BBBB"}));
    state.connection_type=CONNECTION_TYPE_DIRECT;state.callsign="EKDK_CTR";
    ON_CALL(*mockAuth,GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));
    ON_CALL(*mockImpl,GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    auto context=AddMembership(*svc,1,"AAAA","master");
    protobuf::wire::Envelope removed;*removed.mutable_context()=context;removed.mutable_membership_removed();svc->ProcessServerMessage(removed.SerializeAsString());
    ASSERT_TRUE(svc->Memberships().members.empty());EXPECT_FALSE(svc->ShouldSend());
    EXPECT_CALL(*mockImpl,Disconnect()).Times(0);
    EXPECT_CALL(*mockImpl,Send(_)).Times(1).WillOnce(Invoke([](const std::string& bytes){
        protobuf::wire::Envelope e;ASSERT_TRUE(e.ParseFromString(bytes));ASSERT_TRUE(e.has_control());
        EXPECT_EQ(e.control().callsign(),"EKDK_CTR");EXPECT_EQ(e.context().transport_generation(),1u);
        EXPECT_TRUE(e.context().membership_id().empty());EXPECT_EQ(e.control().tuned_frequencies_size(),0);
        EXPECT_THAT(e.control().eligible_airports(), testing::ElementsAre("AAAA", "BBBB"));
    }));
    svc->OnTimer(1);
    auto rejoin=context;rejoin.set_membership_id("new-membership");rejoin.set_generation(2);
    protobuf::wire::Envelope role;*role.mutable_context()=rejoin;role.mutable_session_info()->set_role("master");svc->ProcessServerMessage(role.SerializeAsString());
    EXPECT_TRUE(svc->ShouldSend());EXPECT_TRUE(svc->Memberships().members.contains("new-membership"));
    svc->ProcessServerMessage(removed.SerializeAsString());EXPECT_TRUE(svc->ShouldSend());
}

// ---------------------------------------------------------------------------
// Reconnect timing
// ---------------------------------------------------------------------------

// Extended seam that exposes the two protected members needed for reconnect tests.
class ReconnectSeam : public WebSocketService {
public:
    ReconnectSeam(std::shared_ptr<authentication::IAuthenticationService> a,
                  std::shared_ptr<IFlightStripsPlugin> p,
                  std::shared_ptr<handlers::ConnectionEventHandlers> c,
                  std::shared_ptr<handlers::MessageHandlers> m,
                  std::unique_ptr<WebSocket> ws,
                  std::string localIp = "")
        : WebSocketService(std::move(a), std::move(p), std::move(c),
                           std::move(m), std::move(ws), /*enabled=*/true, std::move(localIp)) {}

    void BeginConnected() { OnConnected(); }
    void FinishConnected() { ProcessConnected(); }
    void SimulateConnected(MockWebSocketImpl& transport) {
        const auto previous = transport.GetStatus();
        ON_CALL(transport, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
        OnConnected();
        ProcessConnected();
        ON_CALL(transport, GetStatus()).WillByDefault(Return(previous));
    }

    void BackdateOnlineSince(int seconds) {
        online_since_ =
            std::chrono::steady_clock::now() - std::chrono::seconds(seconds);
    }
};

class WebSocketServiceReconnectTest : public ::testing::Test {
protected:
    ConnectionState state{};
    MockWebSocketImpl* mockImpl{nullptr};

    std::shared_ptr<NiceMock<MockAuthenticationService>>  mockAuth;
    std::shared_ptr<NiceMock<MockFlightStripsPlugin>>     mockPlugin;
    std::shared_ptr<handlers::ConnectionEventHandlers>    connHandlers;
    std::shared_ptr<handlers::MessageHandlers>            msgHandlers;

    ReconnectSeam* svc{nullptr};
    std::unique_ptr<ReconnectSeam> svcOwner;

    void SetUp() override {
        mockAuth     = std::make_shared<NiceMock<MockAuthenticationService>>();
        mockPlugin   = std::make_shared<NiceMock<MockFlightStripsPlugin>>();
        connHandlers = std::make_shared<handlers::ConnectionEventHandlers>();
        msgHandlers  = std::make_shared<handlers::MessageHandlers>();

        ON_CALL(*mockPlugin, GetConnectionState()).WillByDefault(ReturnRef(state));
        ON_CALL(*mockAuth,   GetAuthenticationState()).WillByDefault(Return(NONE));
        ON_CALL(*mockAuth,   GetAccessToken()).WillByDefault(Return(""));

        auto implOwned = std::make_unique<NiceMock<MockWebSocketImpl>>();
        mockImpl = implOwned.get();
        ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_DISCONNECTED));

        auto ws = std::unique_ptr<WebSocket>(new WebSocket(std::move(implOwned)));
        svcOwner = std::make_unique<ReconnectSeam>(
            mockAuth, mockPlugin, connHandlers, msgHandlers, std::move(ws));
        svc = svcOwner.get();
    }

    void SetShouldConnect() {
        state.primary_frequency = "121.500";
        state.relevant_airport  = "EKCH";
        state.connection_type   = CONNECTION_TYPE_DIRECT;
        ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));
    }
};

TEST_F(WebSocketServiceReconnectTest, ReconnectFencesOldMembershipCallbacksUntilSdkInitialization) {
    SetShouldConnect();
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    SetTestRole(*svc, STATE_MASTER);
    EXPECT_TRUE(svc->ShouldSend());
    svc->BeginConnected();
    // Authentication announces the new token before updating its provider cache.
    svc->OnTokenUpdate("fresh-token");
    EXPECT_FALSE(svc->IsConnected()); EXPECT_FALSE(svc->ShouldSend());
    EXPECT_CALL(*mockImpl, Send(_)).Times(0);
    svc->SendEvent(HoldEvent("SAS123", "OLPIB", "enroute", "1422"));
    svc->SendEvent(ControllerOnlineEvent("121.500", "AAAA_TWR"));
    ::testing::Mock::VerifyAndClearExpectations(mockImpl);
    std::vector<protobuf::wire::Envelope> sent;
    EXPECT_CALL(*mockImpl, Send(_)).Times(2).WillRepeatedly(Invoke([&](const std::string& bytes) {
        protobuf::wire::Envelope event; ASSERT_TRUE(event.ParseFromString(bytes)); sent.push_back(event);
    }));
    svc->FinishConnected();
    EXPECT_TRUE(svc->IsConnected()); EXPECT_FALSE(svc->ShouldSend());
    EXPECT_TRUE(svc->Memberships().members.empty());
    ASSERT_EQ(sent.size(), 2u); EXPECT_TRUE(sent[0].has_token()); EXPECT_TRUE(sent[1].has_control());
    EXPECT_EQ(sent[0].token().token(), "fresh-token");
}

TEST_F(WebSocketServiceReconnectTest, SdkTimerPreservesPendingInitializationUntilPhysicalOpenCompletes) {
    SetShouldConnect();
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTING));
    EXPECT_CALL(*mockImpl, Send(_)).Times(0);
    svc->OnTokenUpdate("connecting-token");
    svc->OnTimer(1); // Keep the announced token while authentication's provider cache is still stale.
    svc->BeginConnected();
    svc->FinishConnected();
    ::testing::Mock::VerifyAndClearExpectations(mockImpl);
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    EXPECT_FALSE(svc->IsConnected());
    std::vector<protobuf::wire::Envelope> sent;
    EXPECT_CALL(*mockImpl, Send(_)).Times(2).WillRepeatedly(Invoke([&](const std::string& bytes) {
        protobuf::wire::Envelope event; ASSERT_TRUE(event.ParseFromString(bytes)); sent.push_back(event);
    }));
    svc->FinishConnected();
    EXPECT_TRUE(svc->IsConnected());
    ASSERT_EQ(sent.size(), 2u); ASSERT_TRUE(sent[0].has_token()); EXPECT_EQ(sent[0].token().token(), "connecting-token");
}

// Conditions were lost while the WebSocket was connected; next reconnect uses the fast delay.
TEST_F(WebSocketServiceReconnectTest, OnTimer_AfterConditionsLost_NextConnectUsesFastDelay) {
    SetShouldConnect();
    svc->SimulateConnected(*mockImpl);
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    state.relevant_airport = "";          // lose airport while connected
    svc->OnTimer(1);                      // → Disconnect()

    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_DISCONNECTED));
    state.relevant_airport = "EKCH";     // conditions restored

    EXPECT_CALL(*mockImpl, Connect()).Times(0);
    svc->OnTimer(1);
    const auto delay = svc->GetDelaySecondsRemaining();
    ASSERT_TRUE(delay.has_value());
    EXPECT_GE(*delay, 4);
    EXPECT_LE(*delay, 5);
}

// WebSocket drops while ES conditions are still met (server restart / blip); reconnect uses the fast delay.
TEST_F(WebSocketServiceReconnectTest, OnTimer_AfterServerDrop_ConnectUsesFastDelay) {
    SetShouldConnect();
    svc->SimulateConnected(*mockImpl);

    // Status drops to DISCONNECTED; conditions unchanged.
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_DISCONNECTED));

    EXPECT_CALL(*mockImpl, Connect()).Times(0);
    svc->OnTimer(1);
    const auto delay = svc->GetDelaySecondsRemaining();
    ASSERT_TRUE(delay.has_value());
    EXPECT_GE(*delay, 4);
    EXPECT_LE(*delay, 5);
}

// Primary selected after ≥30 s online: connect immediately.
TEST_F(WebSocketServiceReconnectTest, OnTimer_PrimarySelectedAfter30sOnline_ConnectsImmediately) {
    state.relevant_airport = "EKCH";
    state.connection_type  = CONNECTION_TYPE_DIRECT;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));
    svc->BackdateOnlineSince(35);

    state.primary_frequency = "121.500"; // primary now selected

    EXPECT_CALL(*mockImpl, Connect()).Times(1);
    svc->OnTimer(1);
    EXPECT_FALSE(svc->GetDelaySecondsRemaining().has_value());
}

// Primary selected after <30 s online: wait for the remainder of the 30 s online-transition delay.
TEST_F(WebSocketServiceReconnectTest, OnTimer_PrimarySelectedBefore30sOnline_StillDelayed) {
    state.relevant_airport = "EKCH";
    state.connection_type  = CONNECTION_TYPE_DIRECT;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));
    svc->BackdateOnlineSince(20);

    state.primary_frequency = "121.500"; // primary now selected

    EXPECT_CALL(*mockImpl, Connect()).Times(0);
    svc->OnTimer(1);
    const auto delay = svc->GetDelaySecondsRemaining();
    ASSERT_TRUE(delay.has_value());
    EXPECT_GE(*delay, 9);
    EXPECT_LE(*delay, 10);
}

// ---------------------------------------------------------------------------
// Backoff on repeated connection failures
// ---------------------------------------------------------------------------

// First failure schedules a backoff delay; Connect() is not called again immediately.
TEST_F(WebSocketServiceReconnectTest, OnTimer_FirstConnectFailure_SetsBackoffAndNoCalls) {
    SetShouldConnect();
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_FAILED));

    EXPECT_CALL(*mockImpl, Connect()).Times(0);
    svc->OnTimer(1);
    EXPECT_TRUE(svc->IsBackingOff());
}

// While a backoff delay is pending, both IsBackingOff and IsPendingConnect are true.
TEST_F(WebSocketServiceReconnectTest, OnTimer_WhileBackingOff_StateFlags) {
    SetShouldConnect();
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_FAILED));
    svc->OnTimer(1);

    EXPECT_TRUE(svc->IsBackingOff());
    EXPECT_TRUE(svc->IsPendingConnect());
}

// Repeated ticks during a backoff delay do not trigger a retry.
TEST_F(WebSocketServiceReconnectTest, OnTimer_BackoffDelayPending_DoesNotRetryEarly) {
    SetShouldConnect();
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_FAILED));
    svc->OnTimer(1);  // schedules 500 ms backoff

    EXPECT_CALL(*mockImpl, Connect()).Times(0);
    svc->OnTimer(1);
    svc->OnTimer(1);
}

// Successful connection resets backoff; next failure restarts from the first table step.
TEST_F(WebSocketServiceReconnectTest, OnTimer_SuccessAfterFailure_ResetsBackoff) {
    SetShouldConnect();
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_FAILED));
    svc->OnTimer(1);
    EXPECT_TRUE(svc->IsBackingOff());

    svc->SimulateConnected(*mockImpl);           // resets fail_count_ and connect_after_
    EXPECT_FALSE(svc->IsBackingOff());

    // Next failure reschedules from the first step (500 ms).
    svc->OnTimer(1);
    EXPECT_TRUE(svc->IsBackingOff());
}

// Conditions dropping while backing off resets fail count; reconnect returns to the online-transition delay.
TEST_F(WebSocketServiceReconnectTest, OnTimer_AuthenticationLostDuringBackoff_ResetsFailCount) {
    SetShouldConnect();
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_FAILED));
    svc->OnTimer(1);
    EXPECT_TRUE(svc->IsBackingOff());

    // Conditions drop while backing off — triggers intentional disconnect path.
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(NONE));
    svc->OnTimer(1);

    // Conditions restored; socket DISCONNECTED — expect the online-transition delay and no backoff.
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_DISCONNECTED));
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));

    EXPECT_FALSE(svc->IsBackingOff());
    EXPECT_CALL(*mockImpl, Connect()).Times(0);
    svc->OnTimer(1);
    const auto delay = svc->GetDelaySecondsRemaining();
    ASSERT_TRUE(delay.has_value());
    EXPECT_GE(*delay, 29);
    EXPECT_LE(*delay, 30);
}

TEST_F(WebSocketServiceReconnectTest, OnConnected_DirectConnection_UsesSelectedSweatboxSessionInLoginEvent) {
    state.range = 150;
    state.connection_type = CONNECTION_TYPE_DIRECT;
    state.primary_frequency = "121.500";
    state.callsign = "EKCH_GND";
    state.relevant_airport = "EKCH";
    state.prefer_sweatbox = true;
    ON_CALL(*mockAuth, GetAccessToken()).WillByDefault(Return("token-123"));

    std::vector<flightstrips::euroscope::v1::Envelope> sent;
    EXPECT_CALL(*mockImpl, Send(_)).Times(2).WillRepeatedly(Invoke([&sent](const std::string& payload) {
        flightstrips::euroscope::v1::Envelope envelope;
        EXPECT_TRUE(envelope.ParseFromString(payload));
        sent.push_back(std::move(envelope));
    }));

    svc->SimulateConnected(*mockImpl);

    ASSERT_EQ(sent.size(), 2u);
    EXPECT_TRUE(sent[0].has_token());
    EXPECT_EQ(sent[0].token().version(), PLUGIN_VERSION);
    EXPECT_TRUE(sent[1].has_control());
    EXPECT_EQ(sent[1].control().environment(), "SWEATBOX");
}

TEST_F(WebSocketServiceReconnectTest, OnConnected_PlaybackConnection_OverridesManualPreferenceInLoginEvent) {
    state.range = 150;
    state.connection_type = CONNECTION_TYPE_PLAYBACK;
    state.primary_frequency = "121.500";
    state.callsign = "EKCH_GND";
    state.relevant_airport = "EKCH";
    state.prefer_sweatbox = false;
    ON_CALL(*mockAuth, GetAccessToken()).WillByDefault(Return("token-123"));

    std::vector<flightstrips::euroscope::v1::Envelope> sent;
    EXPECT_CALL(*mockImpl, Send(_)).Times(2).WillRepeatedly(Invoke([&sent](const std::string& payload) {
        flightstrips::euroscope::v1::Envelope envelope;
        EXPECT_TRUE(envelope.ParseFromString(payload));
        sent.push_back(std::move(envelope));
    }));

    svc->SimulateConnected(*mockImpl);

    ASSERT_EQ(sent.size(), 2u);
    EXPECT_TRUE(sent[1].has_control());
    const auto& name = sent[1].control().environment();
    EXPECT_THAT(name, ::testing::StartsWith("PLAYBACK_"));
    EXPECT_EQ(name.size(), 41u);
    EXPECT_EQ(name.find_first_not_of("0123456789ABCDEF", 9), std::string::npos);
}

TEST_F(WebSocketServiceReconnectTest, PlaybackSessionSurvivesBackendAndManualReconnects) {
    SetShouldConnect();
    state.connection_type = CONNECTION_TYPE_PLAYBACK;
    std::vector<std::string> names;
    EXPECT_CALL(*mockImpl, Send(_)).WillRepeatedly(Invoke([&names](const std::string& payload) {
        flightstrips::euroscope::v1::Envelope envelope;
        ASSERT_TRUE(envelope.ParseFromString(payload));
        if (envelope.has_control()) names.push_back(envelope.control().environment());
    }));

    svc->OnTimer(1);
    svc->SimulateConnected(*mockImpl);
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    EXPECT_CALL(*mockImpl, Disconnect()).Times(1);
    svc->OnTimer(1); // The playback ID must not be mistaken for a mode change.
    state.primary_frequency = "121.600";
    svc->OnTimer(2); // Position changes must reuse the same playback ID.

    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_DISCONNECTED));
    svc->OnTimer(3); // Backend restart while EuroScope continues playback.
    svc->SimulateConnected(*mockImpl);
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    svc->Reconnect();
    svc->SimulateConnected(*mockImpl);

    ASSERT_EQ(names.size(), 4u);
    for (const auto& name : names) EXPECT_EQ(name, names.front());
}

TEST_F(WebSocketServiceReconnectTest, PlaybackSessionResetsAfterStopWhileBackendIsDisconnected) {
    SetShouldConnect();
    state.connection_type = CONNECTION_TYPE_PLAYBACK;
    std::vector<std::string> names;
    EXPECT_CALL(*mockImpl, Send(_)).WillRepeatedly(Invoke([&names](const std::string& payload) {
        flightstrips::euroscope::v1::Envelope envelope;
        ASSERT_TRUE(envelope.ParseFromString(payload));
        if (envelope.has_control()) names.push_back(envelope.control().environment());
    }));

    svc->SimulateConnected(*mockImpl);
    state.relevant_airport.clear();
    svc->OnTimer(1); // Losing backend connection prerequisites is not a playback stop.
    state.relevant_airport = "EKCH";
    svc->SimulateConnected(*mockImpl);

    state.connection_type = CONNECTION_TYPE_NO;
    svc->OnTimer(2); // Stop while the backend remains disconnected.
    state.connection_type = CONNECTION_TYPE_PLAYBACK;
    svc->OnTimer(3);
    svc->SimulateConnected(*mockImpl);

    ASSERT_EQ(names.size(), 3u);
    EXPECT_EQ(names[0], names[1]);
    EXPECT_NE(names[0], names[2]);
}

TEST_F(WebSocketServiceReconnectTest, OnConnected_ObserverLoginIncludesObserverFlag) {
    state.range = 150;
    state.connection_type = CONNECTION_TYPE_DIRECT;
    state.primary_frequency = "199.998";
    state.callsign = "EKCH_OBS";
    state.relevant_airport = "EKCH";
    state.observer = true;
    ON_CALL(*mockAuth, GetAccessToken()).WillByDefault(Return("token-123"));

    std::vector<flightstrips::euroscope::v1::Envelope> sent;
    EXPECT_CALL(*mockImpl, Send(_)).Times(2).WillRepeatedly(Invoke([&sent](const std::string& payload) {
        flightstrips::euroscope::v1::Envelope envelope;
        EXPECT_TRUE(envelope.ParseFromString(payload));
        sent.push_back(std::move(envelope));
    }));

    svc->SimulateConnected(*mockImpl);

    ASSERT_EQ(sent.size(), 2u);
    EXPECT_TRUE(sent[1].has_control());
    EXPECT_TRUE(sent[1].control().observer());
}

TEST_F(WebSocketServiceReconnectTest, OnConnected_LoginIncludesLocalIpWhenAvailable) {
    state.range = 150;
    state.connection_type = CONNECTION_TYPE_DIRECT;
    state.primary_frequency = "121.500";
    state.callsign = "EKCH_GND";
    state.relevant_airport = "EKCH";
    ON_CALL(*mockAuth, GetAccessToken()).WillByDefault(Return("token-123"));

    auto implOwned = std::make_unique<NiceMock<MockWebSocketImpl>>();
    mockImpl = implOwned.get();
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_DISCONNECTED));
    auto ws = std::unique_ptr<WebSocket>(new WebSocket(std::move(implOwned)));
    svcOwner = std::make_unique<ReconnectSeam>(mockAuth, mockPlugin, connHandlers, msgHandlers, std::move(ws), "192.168.1.25");
    svc = svcOwner.get();

    std::vector<flightstrips::euroscope::v1::Envelope> sent;
    EXPECT_CALL(*mockImpl, Send(_)).Times(2).WillRepeatedly(Invoke([&sent](const std::string& payload) {
        flightstrips::euroscope::v1::Envelope envelope;
        EXPECT_TRUE(envelope.ParseFromString(payload));
        sent.push_back(std::move(envelope));
    }));

    svc->SimulateConnected(*mockImpl);

    ASSERT_EQ(sent.size(), 2u);
    EXPECT_TRUE(sent[1].has_control());
    EXPECT_EQ(sent[1].control().local_ip(), "192.168.1.25");
}

// ---------------------------------------------------------------------------
// GetStats / ShouldSend accessors
// ---------------------------------------------------------------------------

TEST_F(WebSocketServiceOnTimerTest, ShouldSend_WhenDisconnected_ReturnsFalse) {
    EXPECT_FALSE(svc->ShouldSend());
}

TEST_F(WebSocketServiceOnTimerTest, ShouldSend_WhenConnectedButSlave_ReturnsFalse) {
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    SetTestRole(*svc,STATE_SLAVE);
    EXPECT_FALSE(svc->ShouldSend());
}

TEST_F(WebSocketServiceOnTimerTest, ShouldSend_WhenConnectedButObserver_ReturnsFalse) {
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    SetTestRole(*svc,STATE_OBSERVER);
    EXPECT_FALSE(svc->ShouldSend());
}

TEST_F(WebSocketServiceOnTimerTest, ShouldSend_WhenConnectedAndMaster_ReturnsTrue) {
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    SetTestRole(*svc,STATE_MASTER);
    EXPECT_TRUE(svc->ShouldSend());
}

TEST_F(WebSocketServiceOnTimerTest, GetStats_Initial_AllZero) {
    const auto stats = svc->GetStats();
    EXPECT_EQ(stats.tx,     0);
    EXPECT_EQ(stats.rx,     0);
    EXPECT_EQ(stats.queued, 0);
    EXPECT_EQ(stats.role,   STATE_UNKNOWN);
}

TEST_F(WebSocketServiceOnTimerTest, GetDelaySecondsRemaining_Initial_ReturnsNullopt) {
    EXPECT_FALSE(svc->GetDelaySecondsRemaining().has_value());
}

TEST_F(WebSocketServiceOnTimerTest, IsConnected_Initial_ReturnsFalse) {
    EXPECT_FALSE(svc->IsConnected());
}

TEST_F(WebSocketServiceOnTimerTest, IsPendingConnect_Initial_ReturnsFalse) {
    EXPECT_FALSE(svc->IsPendingConnect());
}

TEST_F(WebSocketServiceOnTimerTest, SendEvent_WithoutMembershipDropsRunwayObservation) {
    state.observer = true;

    EXPECT_CALL(*mockImpl, Send(_)).Times(0);
    svc->SendEvent(RunwayEvent({}));
}

TEST_F(WebSocketServiceOnTimerTest, MasterReplaysUntrackedHoldAfterReconnect) {
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    SetTestRole(*svc,STATE_MASTER);
    auto serviceSocket = std::shared_ptr<WebSocketService>(svc.get(), [](WebSocketService*) {});
    flightplan::FlightPlanService plans(serviceSocket, nullptr, nullptr, nullptr, nullptr);
    EXPECT_CALL(*mockImpl, Send(_)).Times(2).WillRepeatedly(Invoke([](const std::string& bytes) {
        flightstrips::euroscope::v1::Envelope event;
        ASSERT_TRUE(event.ParseFromString(bytes));
        ASSERT_TRUE(event.has_hold());
        EXPECT_EQ(event.hold().callsign(), "SAS123");
        EXPECT_EQ(event.hold().hold(), "OLPIB");
        EXPECT_EQ(event.hold().hold_eat(), "1422");
    }));
    plans.ReplayTrackedHold("SAS123", false, {true, false, "OLPIB"}, "1422");
    plans.ReplayTrackedHold("SAS123", false, {}, {}); // Cached hold survives absent remote annotation.
    SetTestRole(*svc,STATE_SLAVE);
    plans.ReplayTrackedHold("SAS123", false, {true, false, "OLPIB"}, {});
}

TEST_F(WebSocketServiceOnTimerTest, ShouldProcessServerMessageType_WhenObserver_AllowsMismatchAlerts) {
    state.observer = true;

    EXPECT_TRUE(svc->ShouldProcessServerMessageType(EVENT_SESSION_INFO_NAME));
    EXPECT_TRUE(svc->ShouldProcessServerMessageType(EVENT_MASTER_INFO_NAME));
    EXPECT_TRUE(svc->ShouldProcessServerMessageType(EVENT_RUNWAY_MISMATCH_ALERT_NAME));
    EXPECT_FALSE(svc->ShouldProcessServerMessageType(EVENT_BACKEND_SYNC_NAME));
}

TEST_F(WebSocketServiceOnTimerTest, MasterMetadataDoesNotChangeRoleAndClearsOnReconnect) {
    SetTestRole(*svc,STATE_SLAVE);
    svc->SetMasterCallsign("EKDK_FMP");
    EXPECT_EQ(svc->GetStats().masterCallsign, "EKDK_FMP");
    EXPECT_EQ(svc->GetStats().role, STATE_SLAVE);
    svc->Reconnect();
    EXPECT_TRUE(svc->GetStats().masterCallsign.empty());
}

// ---------------------------------------------------------------------------
// ClientState enum
// ---------------------------------------------------------------------------

TEST(ClientStateTest, Values_AreDistinct) {
    using namespace FlightStrips::websocket;
    EXPECT_NE(STATE_UNKNOWN, STATE_SLAVE);
    EXPECT_NE(STATE_SLAVE,   STATE_MASTER);
    EXPECT_NE(STATE_UNKNOWN, STATE_MASTER);
    EXPECT_NE(STATE_OBSERVER, STATE_MASTER);
    EXPECT_NE(STATE_OBSERVER, STATE_SLAVE);
}

// ---------------------------------------------------------------------------
// Stats struct defaults
// ---------------------------------------------------------------------------

TEST(StatsTest, DefaultConstruction_TxIsZero) {
    FlightStrips::websocket::Stats s;
    EXPECT_EQ(s.tx, 0);
}

TEST(StatsTest, DefaultConstruction_RxIsZero) {
    FlightStrips::websocket::Stats s;
    EXPECT_EQ(s.rx, 0);
}

TEST(StatsTest, DefaultConstruction_QueuedIsZero) {
    FlightStrips::websocket::Stats s;
    EXPECT_EQ(s.queued, 0);
}

TEST(StatsTest, DefaultConstruction_RoleIsUnknown) {
    FlightStrips::websocket::Stats s;
    EXPECT_EQ(s.role, FlightStrips::websocket::STATE_UNKNOWN);
}

// ---------------------------------------------------------------------------
// Event type serialisation — EventType enum round-trip via nlohmann::json
// ---------------------------------------------------------------------------

using namespace FlightStrips::websocket;

TEST(EventTypeTest, TokenEvent_SerializesCorrectType) {
    TokenEvent e("my-token");
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"], EVENT_TOKEN_NAME);
}

TEST(EventTypeTest, TokenEvent_SerializesToken) {
    TokenEvent e("abc-123");
    const nlohmann::json j = e;
    EXPECT_EQ(j["token"], "abc-123");
}

TEST(EventTypeTest, TokenEvent_SerializesVersion) {
    TokenEvent e("abc-123", "0.16.0");
    const nlohmann::json j = e;
    EXPECT_EQ(j["version"], "0.16.0");
}

TEST(EventTypeTest, LoginEvent_SerializesCorrectType) {
    LoginEvent e("EKCH", "OBS", "GND", "EK_GND", 150, true);
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"], EVENT_LOGIN_NAME);
}

TEST(EventTypeTest, LoginEvent_SerializesAllFields) {
    LoginEvent e("EKCH", "OBS", "GND", "EK_GND", 150, true);
    const nlohmann::json j = e;
    EXPECT_EQ(j["airport"],    "EKCH");
    EXPECT_EQ(j["connection"], "OBS");
    EXPECT_EQ(j["position"],   "GND");
    EXPECT_EQ(j["callsign"],   "EK_GND");
    EXPECT_EQ(j["range"],      150);
    EXPECT_EQ(j["observer"],   true);
}

TEST(EventTypeTest, LoginEvent_SerializesLocalIpWhenPresent) {
    LoginEvent e("EKCH", "OBS", "GND", "EK_GND", 150, true, "192.168.1.25");
    const nlohmann::json j = e;
    EXPECT_EQ(j["local_ip"], "192.168.1.25");
}

TEST(EventTypeTest, EventType_DeserializesTokenName) {
    const nlohmann::json j = nlohmann::json::parse(R"({"type": "token"})");
    const auto t = j["type"].get<EventType>();
    EXPECT_EQ(t, EVENT_TOKEN);
}

TEST(EventTypeTest, EventType_DeserializesLoginName) {
    const nlohmann::json j = nlohmann::json::parse(R"({"type": "login"})");
    const auto t = j["type"].get<EventType>();
    EXPECT_EQ(t, EVENT_LOGIN);
}

TEST(EventTypeTest, EventType_UnknownStringDeserializesToUnknown) {
    const nlohmann::json j = nlohmann::json::parse(R"({"type": "not_a_real_event"})");
    const auto t = j["type"].get<EventType>();
    EXPECT_EQ(t, EVENT_UNKNOWN);
}

// ---------------------------------------------------------------------------
// RunwayEvent serialisation
// ---------------------------------------------------------------------------

TEST(RunwayEventTest, EmptyRunways_SerializesCorrectly) {
    RunwayEvent e{{}};
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"], EVENT_RUNWAY_NAME);
    EXPECT_TRUE(j["runways"].is_array());
    EXPECT_TRUE(j["runways"].empty());
}

TEST(RunwayEventTest, WithRunways_SerializesRunwayFields) {
    Runway r;
    r.name = "22L";
    r.departure = true;
    r.arrival = false;

    RunwayEvent e{std::vector<Runway>{r}};

    const nlohmann::json j = e;
    ASSERT_EQ(j["runways"].size(), 1u);
    EXPECT_EQ(j["runways"][0]["name"],      "22L");
    EXPECT_EQ(j["runways"][0]["departure"], true);
    EXPECT_EQ(j["runways"][0]["arrival"],   false);
}

// ---------------------------------------------------------------------------
// SquawkEvent serialisation
// ---------------------------------------------------------------------------

TEST(SquawkEventTest, Serializes_Callsign_And_Squawk) {
    SquawkEvent e("EKS123", "7700");
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],     EVENT_SQUAWK_NAME);
    EXPECT_EQ(j["callsign"], "EKS123");
    EXPECT_EQ(j["squawk"],   "7700");
}

// ---------------------------------------------------------------------------
// AssignedSquawkEvent serialisation
// ---------------------------------------------------------------------------

TEST(AssignedSquawkEventTest, Serializes_Callsign_And_Squawk) {
    AssignedSquawkEvent e("EKS100", "2200");
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],     EVENT_ASSIGNED_SQUAWK_NAME);
    EXPECT_EQ(j["callsign"], "EKS100");
    EXPECT_EQ(j["squawk"],   "2200");
}

TEST(AssignedSquawkEventTest, DefaultConstruction_DoesNotCrash) {
    AssignedSquawkEvent e;
    const nlohmann::json j = e;
    SUCCEED(); // default ctor leaves type as EVENT_UNKNOWN — just verify no crash
}

// ---------------------------------------------------------------------------
// HeadingEvent serialisation
// ---------------------------------------------------------------------------

TEST(HeadingEventTest, Serializes_Callsign_And_Heading) {
    HeadingEvent e("EKS200", 270);
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],     EVENT_HEADING_NAME);
    EXPECT_EQ(j["callsign"], "EKS200");
    EXPECT_EQ(j["heading"],  270);
}

TEST(HeadingEventTest, DefaultConstruction_DoesNotCrash) {
    HeadingEvent e;
    const nlohmann::json j = e;
    SUCCEED(); // default ctor leaves type as EVENT_UNKNOWN
}

// ---------------------------------------------------------------------------
// RequestedAltitudeEvent serialisation
// ---------------------------------------------------------------------------

TEST(RequestedAltitudeEventTest, Serializes_Callsign_And_Altitude) {
    RequestedAltitudeEvent e("EKS300", 10000);
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],     EVENT_REQUESTED_ALTITUDE_NAME);
    EXPECT_EQ(j["callsign"], "EKS300");
    EXPECT_EQ(j["altitude"], 10000);
}

TEST(RequestedAltitudeEventTest, DefaultConstruction_DoesNotCrash) {
    RequestedAltitudeEvent e;
    const nlohmann::json j = e;
    SUCCEED(); // default ctor leaves type as EVENT_UNKNOWN
}

// ---------------------------------------------------------------------------
// ClearedAltitudeEvent serialisation
// ---------------------------------------------------------------------------

TEST(ClearedAltitudeEventTest, Serializes_Callsign_And_Altitude) {
    ClearedAltitudeEvent e("EKS400", 8000);
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],     EVENT_CLEARED_ALTITUDE_NAME);
    EXPECT_EQ(j["callsign"], "EKS400");
    EXPECT_EQ(j["altitude"], 8000);
}

TEST(ClearedAltitudeEventTest, DefaultConstruction_DoesNotCrash) {
    ClearedAltitudeEvent e;
    const nlohmann::json j = e;
    SUCCEED(); // default ctor leaves type as EVENT_UNKNOWN
}

// ---------------------------------------------------------------------------
// CommunicationTypeEvent serialisation
// ---------------------------------------------------------------------------

TEST(CommunicationTypeEventTest, Serializes_Callsign_And_Type) {
    CommunicationTypeEvent e("EKS500", 'V');
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],               EVENT_COMMUNICATION_TYPE_NAME);
    EXPECT_EQ(j["callsign"],           "EKS500");
    EXPECT_EQ(j["communication_type"], "V");
}

TEST(CommunicationTypeEventTest, DefaultConstruction_DoesNotCrash) {
    CommunicationTypeEvent e;
    const nlohmann::json j = e;
    SUCCEED(); // default ctor leaves type as EVENT_UNKNOWN
}

// ---------------------------------------------------------------------------
// GroundStateEvent serialisation
// ---------------------------------------------------------------------------

TEST(GroundStateEventTest, Serializes_Callsign_And_State) {
    GroundStateEvent e("EKS600", "TAXI");
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],         EVENT_GROUND_STATE_NAME);
    EXPECT_EQ(j["callsign"],     "EKS600");
    EXPECT_EQ(j["ground_state"], "TAXI");
}

TEST(GroundStateEventTest, DefaultConstruction_DoesNotCrash) {
    GroundStateEvent e;
    const nlohmann::json j = e;
    SUCCEED(); // default ctor leaves type as EVENT_UNKNOWN
}

// ---------------------------------------------------------------------------
// ClearedFlagEvent serialisation
// ---------------------------------------------------------------------------

TEST(ClearedFlagEventTest, Serializes_Callsign_And_Cleared_True) {
    ClearedFlagEvent e("EKS700", true);
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],     EVENT_CLEARED_FLAG_NAME);
    EXPECT_EQ(j["callsign"], "EKS700");
    EXPECT_EQ(j["cleared"],  true);
}

TEST(ClearedFlagEventTest, Serializes_Cleared_False) {
    ClearedFlagEvent e("EKS701", false);
    const nlohmann::json j = e;
    EXPECT_EQ(j["cleared"], false);
}

TEST(ClearedFlagEventTest, DefaultConstruction_DoesNotCrash) {
    ClearedFlagEvent e;
    const nlohmann::json j = e;
    SUCCEED(); // default ctor leaves type as EVENT_UNKNOWN
}

// ---------------------------------------------------------------------------
// PositionEvent serialisation
// ---------------------------------------------------------------------------

TEST(PositionEventTest, Serializes_AllFields) {
    PositionEvent e("EKS800", 55.6, 12.6, 5000);
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],     EVENT_AIRCRAFT_POSITION_UPDATE_NAME);
    EXPECT_EQ(j["callsign"], "EKS800");
    EXPECT_DOUBLE_EQ(j["lat"].get<double>(), 55.6);
    EXPECT_DOUBLE_EQ(j["lon"].get<double>(), 12.6);
    EXPECT_EQ(j["altitude"], 5000);
}

// ---------------------------------------------------------------------------
// Position struct serialisation
// ---------------------------------------------------------------------------

TEST(PositionStructTest, Serializes_LatLonAltitude) {
    Position p(55.0, 12.0, 3000);
    const nlohmann::json j = p;
    EXPECT_DOUBLE_EQ(j["lat"].get<double>(), 55.0);
    EXPECT_DOUBLE_EQ(j["lon"].get<double>(), 12.0);
    EXPECT_EQ(j["altitude"], 3000);
}

// ---------------------------------------------------------------------------
// ControllerOnlineEvent serialisation
// ---------------------------------------------------------------------------

TEST(ControllerOnlineEventTest, Serializes_Callsign_And_Position) {
    ControllerOnlineEvent e("EKCH_GND", "GND");
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],     EVENT_CONTROLLER_ONLINE_NAME);
    EXPECT_EQ(j["callsign"], "EKCH_GND");
    EXPECT_EQ(j["position"], "GND");
}

// ---------------------------------------------------------------------------
// ControllerOfflineEvent serialisation
// ---------------------------------------------------------------------------

TEST(ControllerOfflineEventTest, Serializes_Callsign) {
    ControllerOfflineEvent e("EKCH_GND");
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],     EVENT_CONTROLLER_OFFLINE_NAME);
    EXPECT_EQ(j["callsign"], "EKCH_GND");
}

// ---------------------------------------------------------------------------
// StandEvent serialisation
// ---------------------------------------------------------------------------

TEST(StandEventTest, Serializes_Callsign_And_Stand) {
    StandEvent e("EKS900", "A1");
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],     EVENT_STAND_NAME);
    EXPECT_EQ(j["callsign"], "EKS900");
    EXPECT_EQ(j["stand"],    "A1");
}

TEST(StandEventTest, DefaultConstruction_DoesNotCrash) {
    StandEvent e;
    const nlohmann::json j = e;
    SUCCEED(); // default ctor leaves type as EVENT_UNKNOWN
}

// ---------------------------------------------------------------------------
// TrackingControllerChangedEvent serialisation
// ---------------------------------------------------------------------------

TEST(TrackingControllerChangedEventTest, Serializes_AllFields) {
    TrackingControllerChangedEvent e("EKS950", "EKCH_CTR");
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],                EVENT_TRACKING_CONTROLLER_CHANGED_NAME);
    EXPECT_EQ(j["callsign"],            "EKS950");
    EXPECT_EQ(j["tracking_controller"], "EKCH_CTR");
}

// ---------------------------------------------------------------------------
// SessionInfoEvent serialisation
// ---------------------------------------------------------------------------

TEST(SessionInfoEventTest, Serializes_Role) {
    SessionInfoEvent e("master");
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"], EVENT_SESSION_INFO_NAME);
    EXPECT_EQ(j["role"], "master");
}

TEST(SessionInfoEventTest, DefaultConstruction_DoesNotCrash) {
    SessionInfoEvent e;
    const nlohmann::json j = e;
    SUCCEED(); // default ctor leaves type as EVENT_UNKNOWN
}

// ---------------------------------------------------------------------------
// GenerateSquawkEvent serialisation
// ---------------------------------------------------------------------------

TEST(GenerateSquawkEventTest, Serializes_Callsign) {
    GenerateSquawkEvent e("EKS001");
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],     EVENT_GENERATE_SQUAWK_NAME);
    EXPECT_EQ(j["callsign"], "EKS001");
}

TEST(GenerateSquawkEventTest, DefaultConstruction_DoesNotCrash) {
    GenerateSquawkEvent e;
    const nlohmann::json j = e;
    SUCCEED(); // default ctor leaves type as EVENT_UNKNOWN
}

// ---------------------------------------------------------------------------
// RouteEvent serialisation
// ---------------------------------------------------------------------------

TEST(RouteEventTest, Serializes_Callsign_And_Route) {
    RouteEvent e("EKS002", "EKCH DCT ESSA");
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],     EVENT_ROUTE_NAME);
    EXPECT_EQ(j["callsign"], "EKS002");
    EXPECT_EQ(j["route"],    "EKCH DCT ESSA");
}

TEST(RouteEventTest, DefaultConstruction_DoesNotCrash) {
    RouteEvent e;
    const nlohmann::json j = e;
    SUCCEED(); // default ctor leaves type as EVENT_UNKNOWN
}

// ---------------------------------------------------------------------------
// RemarksEvent serialisation
// ---------------------------------------------------------------------------

TEST(RemarksEventTest, Serializes_Callsign_And_Remarks) {
    RemarksEvent e("EKS003", "PBN/B2");
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],     EVENT_REMARKS_NAME);
    EXPECT_EQ(j["callsign"], "EKS003");
    EXPECT_EQ(j["remarks"],  "PBN/B2");
}

TEST(RemarksEventTest, DefaultConstruction_DoesNotCrash) {
    RemarksEvent e;
    const nlohmann::json j = e;
    SUCCEED(); // default ctor leaves type as EVENT_UNKNOWN
}

// ---------------------------------------------------------------------------
// SidEvent serialisation
// ---------------------------------------------------------------------------

TEST(SidEventTest, Serializes_Callsign_And_Sid) {
    SidEvent e("EKS004", "BETOS1B");
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],     EVENT_SID_NAME);
    EXPECT_EQ(j["callsign"], "EKS004");
    EXPECT_EQ(j["sid"],      "BETOS1B");
}

TEST(SidEventTest, DefaultConstruction_DoesNotCrash) {
    SidEvent e;
    const nlohmann::json j = e;
    SUCCEED(); // default ctor leaves type as EVENT_UNKNOWN
}

// ---------------------------------------------------------------------------
// AircraftRunwayEvent serialisation
// ---------------------------------------------------------------------------

TEST(AircraftRunwayEventTest, Serializes_Callsign_And_Runway) {
    AircraftRunwayEvent e("EKS005", "22L");
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],     EVENT_AIRCRAFT_RUNWAY_NAME);
    EXPECT_EQ(j["callsign"], "EKS005");
    EXPECT_EQ(j["runway"],   "22L");
}

TEST(AircraftRunwayEventTest, DefaultConstruction_DoesNotCrash) {
    AircraftRunwayEvent e;
    const nlohmann::json j = e;
    SUCCEED(); // default ctor leaves type as EVENT_UNKNOWN
}

// ---------------------------------------------------------------------------
// CoordinationHandoverEvent serialisation
// ---------------------------------------------------------------------------

TEST(CoordinationHandoverEventTest, Serializes_AllFields) {
    CoordinationHandoverEvent e("EKS006", "EKCH_CTR");
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],            EVENT_COORDINATION_HANDOVER_NAME);
    EXPECT_EQ(j["callsign"],        "EKS006");
    EXPECT_EQ(j["target_callsign"], "EKCH_CTR");
}

TEST(CoordinationHandoverEventTest, DefaultConstruction_DoesNotCrash) {
    CoordinationHandoverEvent e;
    const nlohmann::json j = e;
    SUCCEED(); // default ctor leaves type as EVENT_UNKNOWN
}

// ---------------------------------------------------------------------------
// CoordinationReceivedEvent serialisation
// ---------------------------------------------------------------------------

TEST(CoordinationReceivedEventTest, Serializes_AllFields) {
    CoordinationReceivedEvent e("EKS007", "EKCH_APP", "EKCH_A_TWR");
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],                      EVENT_COORDINATION_RECEIVED_NAME);
    EXPECT_EQ(j["callsign"],                  "EKS007");
    EXPECT_EQ(j["source_controller_callsign"], "EKCH_APP");
    EXPECT_EQ(j["controller_callsign"],       "EKCH_A_TWR");
}

// ---------------------------------------------------------------------------
// SidEntry struct serialisation
// ---------------------------------------------------------------------------

TEST(SidEntryTest, Serializes_Name_And_Runway) {
    SidEntry s;
    s.name   = "BETOS1B";
    s.runway = "22L";
    const nlohmann::json j = s;
    EXPECT_EQ(j["name"],   "BETOS1B");
    EXPECT_EQ(j["runway"], "22L");
}

// ---------------------------------------------------------------------------
// BackendSyncStrip deserialization
// ---------------------------------------------------------------------------

TEST(BackendSyncStripTest, Deserializes_AllFields) {
    const nlohmann::json j = nlohmann::json::parse(R"({
        "callsign": "EKS010",
        "assigned_squawk": "2201",
        "cleared": true,
        "ground_state": "PUSH",
        "stand": "B5",
        "pdc_state": "REQUESTED",
        "pdc_request_remarks": "NO SID",
        "hold": "OLPIB",
        "hold_type": "enroute",
        "hold_eat": "1422"
    })");
    const auto s = j.get<BackendSyncStrip>();
    EXPECT_EQ(s.callsign,        "EKS010");
    EXPECT_EQ(s.assigned_squawk, "2201");
    EXPECT_EQ(s.cleared,         true);
    EXPECT_EQ(s.ground_state,    "PUSH");
    EXPECT_EQ(s.stand,           "B5");
    EXPECT_EQ(s.pdc_state,       "REQUESTED");
    EXPECT_EQ(s.pdc_request_remarks, "NO SID");
    EXPECT_EQ(s.hold, "OLPIB");
    EXPECT_EQ(s.hold_type, "enroute");
    EXPECT_EQ(s.hold_eat, "1422");
}

TEST(PdcStateChangeEventTest, Serializes_RequestRemarks) {
    const nlohmann::json j = PdcStateChangeEvent{"EKS011", "REQUESTED", "NO SID"};
    EXPECT_EQ(j["type"], EVENT_PDC_STATE_CHANGE_NAME);
    EXPECT_EQ(j["callsign"], "EKS011");
    EXPECT_EQ(j["state"], "REQUESTED");
    EXPECT_EQ(j["pdc_request_remarks"], "NO SID");
}

TEST(IssuePdcClearanceEventTest, Serializes_Remarks) {
    const nlohmann::json j = IssuePdcClearanceEvent{"EKS012", "NO SID"};
    EXPECT_EQ(j["type"], EVENT_ISSUE_PDC_CLEARANCE_NAME);
    EXPECT_EQ(j["callsign"], "EKS012");
    EXPECT_EQ(j["remarks"], "NO SID");
}

// ---------------------------------------------------------------------------
// BackendSyncEvent deserialization
// ---------------------------------------------------------------------------

TEST(BackendSyncEventTest, DefaultConstruction_StripsIsEmpty) {
    BackendSyncEvent e;
    EXPECT_TRUE(e.strips.empty());
}

TEST(BackendSyncEventTest, DefaultConstruction_LatLonAreZero) {
    BackendSyncEvent e;
    EXPECT_DOUBLE_EQ(e.latitude,  0.0);
    EXPECT_DOUBLE_EQ(e.longitude, 0.0);
}

// ---------------------------------------------------------------------------
// SyncEvent serialisation
// ---------------------------------------------------------------------------

TEST(SyncEventTest, EmptyCollections_SerializesCorrectly) {
    SyncEvent e{{}, {}, {}, {}};
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],       EVENT_SYNC_NAME);
    EXPECT_TRUE(j["strips"].is_array());
    EXPECT_TRUE(j["strips"].empty());
    EXPECT_TRUE(j["controllers"].is_array());
    EXPECT_TRUE(j["runways"].is_array());
    EXPECT_TRUE(j["sids"].is_array());
}

TEST_F(WebSocketServiceOnTimerTest, TrackedAircraftFactsRequireTrackingButNotMasterRole) {
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    for (const auto role : {STATE_MASTER, STATE_SLAVE}) {
        SetTestRole(*svc,role);
        EXPECT_TRUE(svc->ShouldSendTrackedAircraft(true));
        EXPECT_FALSE(svc->ShouldSendTrackedAircraft(false));
    }
    for (const auto role : {STATE_UNKNOWN, STATE_OBSERVER}) {
        SetTestRole(*svc,role);
        EXPECT_FALSE(svc->ShouldSendTrackedAircraft(true));
    }
    SetTestRole(*svc,STATE_MASTER);
    state.observer = true;
    EXPECT_FALSE(svc->ShouldSendTrackedAircraft(true));
    EXPECT_FALSE(svc->ShouldSend());
    state.observer = false;
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_DISCONNECTED));
    EXPECT_FALSE(svc->ShouldSendTrackedAircraft(true));
}

TEST_F(WebSocketServiceOnTimerTest, PositionEventsAreSuppressedAtSendBoundaryAfterMasterDemotion) {
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    const PositionEvent position("SAS123", 55.6, 12.6, 5000);
    EXPECT_CALL(*mockImpl, Send(_)).Times(1);
    SetTestRole(*svc,STATE_MASTER);
    svc->SendEvent(position);
    SetTestRole(*svc,STATE_SLAVE);
    svc->SendEvent(position);
    SetTestRole(*svc,STATE_UNKNOWN);
    svc->SendEvent(position);
    SetTestRole(*svc,STATE_OBSERVER);
    svc->SendEvent(position);
}

TEST(StripUpdateEventTest, SerializesSpokenCallsignField) {
    StripUpdateEvent e(
        "WLF166",
        "EKCH",
        "EGLL",
        "",
        "",
        "RMK/CS=WOLFAIR",
        "22L",
        "1234",
        "2345",
        "BETOS1B",
        "LUXAL2A",
        true,
        "TAXI",
        5000,
        7000,
        180,
        "B738",
        "M",
        "WOLFAIR",
        Position{55.6181, 12.6560, 1500},
        "A12",
        "R",
        "1",
        "1200",
        "",
        "EKCH_APP",
        "J"
    );

    const nlohmann::json j = e;
    EXPECT_EQ(j["type"], EVENT_STRIP_UPDATE_NAME);
    EXPECT_EQ(j["spoken_callsign"], "WOLFAIR");
    EXPECT_EQ(j["star"], "LUXAL2A");
    EXPECT_TRUE(j["hold_supported"]);
    EXPECT_EQ(j["hold"], "");
}

TEST(SyncEventTest, PopulatedStripSerializesSpokenCallsignField) {
    SyncEvent e(
        {
            Strip(
                "WLF166",
                "EKCH",
                "EGLL",
                "",
                "",
                "RMK/CS=WOLFAIR",
                "22L",
                "1234",
                "2345",
                "BETOS1B",
                "LUXAL2A",
                true,
                "TAXI",
                5000,
                7000,
                180,
                "B738",
                "M",
                "WOLFAIR",
                Position{55.6181, 12.6560, 1500},
                "A12",
                "R",
                "1",
                "1200",
                "",
                "EKCH_APP",
                "J"
            )
        },
        {Controller("DEL", "EKCH_DEL")},
        {Runway{"22L", true, false}},
        {SidEntry{"BETOS1B", "22L"}}
    );

    const nlohmann::json j = e;
    ASSERT_EQ(j["strips"].size(), 1u);
    EXPECT_EQ(j["strips"][0]["spoken_callsign"], "WOLFAIR");
    EXPECT_EQ(j["strips"][0]["star"], "LUXAL2A");
    EXPECT_TRUE(j["strips"][0]["hold_supported"]);
    EXPECT_EQ(j["strips"][0]["hold"], "");
}

namespace {
class PendingSquawkDispatch final:public handlers::MessageHandler {
    std::weak_ptr<WebSocketService> socket_;
    PendingSquawkQueue& queue_;
public:
    PendingSquawkDispatch(std::shared_ptr<WebSocketService> socket,PendingSquawkQueue& queue):socket_(socket),queue_(queue) {}
    void OnMessages(const std::vector<std::string>& messages) override {
        for(const auto& bytes:messages) {
            protobuf::wire::Envelope event;if(!event.ParseFromString(bytes) || !event.has_generate_squawk()) continue;
            if(auto socket=socket_.lock()) socket->DeferSquawk(queue_,event.generate_squawk().callsign());
        }
    }
};
}
TEST_F(WebSocketServiceOnTimerTest, DeferredSquawkRetainsOwnNonMasterContextThroughRealQueueAndCallback) {
    ON_CALL(*mockImpl,GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    auto socket=std::shared_ptr<WebSocketService>(std::move(svc));
    PendingSquawkQueue queue;msgHandlers->RegisterHandler(std::make_shared<PendingSquawkDispatch>(socket,queue));
    auto context=AddMembership(*socket,1,"AAAA","slave");
    auto other=AddMembership(*socket,2,"BBBB","master");
    protobuf::wire::Envelope otherInventory;*otherInventory.mutable_context()=other;otherInventory.mutable_backend_sync()->add_strips()->set_callsign("SAS125");socket->ProcessServerMessage(otherInventory.SerializeAsString());
    protobuf::wire::Envelope command;*command.mutable_context()=context;command.set_command_origin(protobuf::wire::COMMAND_ORIGIN_HUMAN);command.set_execution_id("squawk-own");command.mutable_generate_squawk()->set_callsign("SAS125");
    socket->ProcessServerMessage(command.SerializeAsString());
    EXPECT_FALSE(queue.TakeReady([](const std::string&){return false;})); // waiting for radar target
    AddMembership(*socket,1,"AAAA","slave",2); // unrelated master election while waiting
    ASSERT_EQ(queue.TakeReady([](const std::string&){return true;}),std::optional<std::string>{"SAS125"});
    int sdkCalls=0;ASSERT_TRUE(queue.Execute("SAS125",[&]{++sdkCalls;}));EXPECT_EQ(sdkCalls,1);
    flightplan::FlightPlanService plans(socket,{},{},{},nullptr);
    EXPECT_TRUE(socket->ShouldSend()); // independent master for the other airport
    EXPECT_CALL(*mockImpl,Send(_)).Times(2).WillRepeatedly(Invoke([&](const std::string& bytes){
        protobuf::wire::Envelope result;ASSERT_TRUE(result.ParseFromString(bytes));
        if(result.context().membership_id()==context.membership_id()) {EXPECT_EQ(result.execution_id(),"squawk-own");EXPECT_EQ(result.command_origin(),protobuf::wire::COMMAND_ORIGIN_HUMAN);}
        else {EXPECT_EQ(result.context().membership_id(),other.membership_id());EXPECT_TRUE(result.execution_id().empty());EXPECT_EQ(result.command_origin(),protobuf::wire::COMMAND_ORIGIN_UNSPECIFIED);}
        EXPECT_EQ(result.assigned_squawk().squawk(),"1234");
    }));
    EXPECT_TRUE(plans.ReportAssignedSquawk("SAS125","1234")); // later SDK callback, outside active message dispatch
    EXPECT_FALSE(plans.ReportAssignedSquawk("SAS125","1234"));
}
TEST_F(WebSocketServiceOnTimerTest, DeferredSquawkValidatesRemovalAndAutomaticEpochBeforeSdkInvocation) {
    ON_CALL(*mockImpl,GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    auto socket=std::shared_ptr<WebSocketService>(std::move(svc));
    PendingSquawkQueue queue;msgHandlers->RegisterHandler(std::make_shared<PendingSquawkDispatch>(socket,queue));
    auto context=AddMembership(*socket,1,"AAAA","master");
    protobuf::wire::Envelope command;*command.mutable_context()=context;command.set_command_origin(protobuf::wire::COMMAND_ORIGIN_AUTOMATIC);command.set_execution_id("automatic-old");command.mutable_generate_squawk()->set_callsign("SAS125");socket->ProcessServerMessage(command.SerializeAsString());
    AddMembership(*socket,1,"AAAA","slave",2);
    EXPECT_FALSE(queue.TakeReady([](const std::string&){return true;}));
    command.set_command_origin(protobuf::wire::COMMAND_ORIGIN_HUMAN);command.set_execution_id("human-removed");socket->ProcessServerMessage(command.SerializeAsString());
    ASSERT_TRUE(queue.TakeReady([](const std::string&){return true;}));
    auto removal=command;removal.mutable_membership_removed();socket->ProcessServerMessage(removal.SerializeAsString());
    int sdkCalls=0;EXPECT_FALSE(queue.Execute("SAS125",[&]{++sdkCalls;}));EXPECT_EQ(sdkCalls,0);
}
