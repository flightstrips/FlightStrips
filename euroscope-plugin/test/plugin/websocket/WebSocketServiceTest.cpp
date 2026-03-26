#include <gtest/gtest.h>
#include <gmock/gmock.h>
#include "websocket/WebSocketService.h"
#include "websocket/Events.h"
#include "mock/MockFlightStripsPlugin.h"
#include "mock/MockAuthenticationService.h"
#include "handlers/ConnectionEventHandlers.h"
#include "handlers/MessageHandlers.h"

using namespace FlightStrips;
using namespace FlightStrips::websocket;
using namespace FlightStrips::authentication;
using ::testing::NiceMock;
using ::testing::Return;
using ::testing::ReturnRef;

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

TEST_F(WebSocketServiceOnTimerTest, OnTimer_EmptyFrequency_DoesNotScheduleConnect) {
    state.primary_frequency = "";
    state.relevant_airport  = "EKCH";
    state.connection_type   = CONNECTION_TYPE_DIRECT;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));

    EXPECT_CALL(*mockImpl, Connect()).Times(0);
    svc->OnTimer(1);
    EXPECT_FALSE(svc->IsPendingConnect());
}

TEST_F(WebSocketServiceOnTimerTest, OnTimer_SpuriousFrequency199_998_DoesNotScheduleConnect) {
    state.primary_frequency = "199.998";
    state.relevant_airport  = "EKCH";
    state.connection_type   = CONNECTION_TYPE_DIRECT;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));

    EXPECT_CALL(*mockImpl, Connect()).Times(0);
    svc->OnTimer(1);
    EXPECT_FALSE(svc->IsPendingConnect());
}

TEST_F(WebSocketServiceOnTimerTest, OnTimer_EmptyAirport_DoesNotScheduleConnect) {
    state.primary_frequency = "121.500";
    state.relevant_airport  = "";
    state.connection_type   = CONNECTION_TYPE_DIRECT;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));

    EXPECT_CALL(*mockImpl, Connect()).Times(0);
    svc->OnTimer(1);
    EXPECT_FALSE(svc->IsPendingConnect());
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

// ---------------------------------------------------------------------------
// First tick when all conditions met — schedules a fast-connect delay
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
    EXPECT_GT(*delay, 0);
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

// ---------------------------------------------------------------------------
// Disconnects when connected but conditions no longer met
// ---------------------------------------------------------------------------

TEST_F(WebSocketServiceOnTimerTest, OnTimer_ConditionsLost_WhileConnected_Disconnects) {
    // Start "connected".
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    // Conditions not met (no airport).
    state.primary_frequency = "121.500";
    state.relevant_airport  = "";
    state.connection_type   = CONNECTION_TYPE_DIRECT;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));

    EXPECT_CALL(*mockImpl, Disconnect()).Times(1);
    svc->OnTimer(1);
}

TEST_F(WebSocketServiceOnTimerTest, OnTimer_ConditionsLost_WhileConnected_ClearsSessionState) {
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    state.primary_frequency = "121.500";
    state.relevant_airport  = "";
    state.connection_type   = CONNECTION_TYPE_DIRECT;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));

    svc->SetSessionState(STATE_MASTER);
    svc->OnTimer(1);
    // After disconnect, ShouldSend() must be false (state reset to UNKNOWN).
    EXPECT_FALSE(svc->ShouldSend());
}

// ---------------------------------------------------------------------------
// Immediate reconnect — connect_readiness_ state-machine transitions
// ---------------------------------------------------------------------------

// Extended seam that exposes the two protected members needed for reconnect tests.
class ReconnectSeam : public WebSocketService {
public:
    ReconnectSeam(std::shared_ptr<authentication::IAuthenticationService> a,
                  std::shared_ptr<IFlightStripsPlugin> p,
                  std::shared_ptr<handlers::ConnectionEventHandlers> c,
                  std::shared_ptr<handlers::MessageHandlers> m,
                  std::unique_ptr<WebSocket> ws)
        : WebSocketService(std::move(a), std::move(p), std::move(c),
                           std::move(m), std::move(ws), /*enabled=*/true) {}

    void SimulateConnected() { OnConnected(); }

    void BackdateOnlineWithoutPrimary(int seconds) {
        online_without_primary_since_ =
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

// Conditions were lost while the WebSocket was connected; next reconnect is immediate.
TEST_F(WebSocketServiceReconnectTest, OnTimer_AfterConditionsLost_NextConnectIsImmediate) {
    SetShouldConnect();
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    state.relevant_airport = "";          // lose airport while connected
    svc->OnTimer(1);                      // → Disconnect(), connect_readiness_ = RECONNECT

    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_DISCONNECTED));
    state.relevant_airport = "EKCH";     // conditions restored

    EXPECT_CALL(*mockImpl, Connect()).Times(1);
    svc->OnTimer(1);
    EXPECT_FALSE(svc->GetDelaySecondsRemaining().has_value());
}

// WebSocket drops while ES conditions are still met (server restart / blip); reconnect is immediate.
TEST_F(WebSocketServiceReconnectTest, OnTimer_AfterServerDrop_ConnectIsImmediate) {
    SetShouldConnect();
    svc->SimulateConnected();            // OnConnected → connect_readiness_ = RECONNECT

    // Status drops to DISCONNECTED; conditions unchanged.
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_DISCONNECTED));

    EXPECT_CALL(*mockImpl, Connect()).Times(1);
    svc->OnTimer(1);
    EXPECT_FALSE(svc->GetDelaySecondsRemaining().has_value());
}

// Primary selected after ≥30 s online without one: connect immediately.
TEST_F(WebSocketServiceReconnectTest, OnTimer_PrimarySelectedAfter30sOnline_ConnectsImmediately) {
    state.relevant_airport = "EKCH";
    state.connection_type  = CONNECTION_TYPE_DIRECT;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));
    svc->BackdateOnlineWithoutPrimary(35);

    state.primary_frequency = "121.500"; // primary now selected

    EXPECT_CALL(*mockImpl, Connect()).Times(1);
    svc->OnTimer(1);
    EXPECT_FALSE(svc->GetDelaySecondsRemaining().has_value());
}

// Primary selected after <30 s online without one: normal 5 s delay still applies.
TEST_F(WebSocketServiceReconnectTest, OnTimer_PrimarySelectedBefore30sOnline_StillDelayed) {
    state.relevant_airport = "EKCH";
    state.connection_type  = CONNECTION_TYPE_DIRECT;
    ON_CALL(*mockAuth, GetAuthenticationState()).WillByDefault(Return(AUTHENTICATED));
    svc->BackdateOnlineWithoutPrimary(20);

    state.primary_frequency = "121.500"; // primary now selected

    EXPECT_CALL(*mockImpl, Connect()).Times(0);
    svc->OnTimer(1);
    EXPECT_TRUE(svc->GetDelaySecondsRemaining().has_value());
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

    svc->SimulateConnected();           // resets fail_count_ and connect_after_
    EXPECT_FALSE(svc->IsBackingOff());

    // Next failure reschedules from the first step (500 ms).
    svc->OnTimer(1);
    EXPECT_TRUE(svc->IsBackingOff());
}

// Conditions dropping while backing off resets fail count; next reconnect is immediate.
TEST_F(WebSocketServiceReconnectTest, OnTimer_ConditionsDropDuringBackoff_ResetsFailCount) {
    SetShouldConnect();
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_FAILED));
    svc->OnTimer(1);
    EXPECT_TRUE(svc->IsBackingOff());

    // Conditions drop while backing off — triggers intentional disconnect path.
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    state.relevant_airport = "";
    svc->OnTimer(1);

    // Conditions restored; socket DISCONNECTED — expect immediate connect, no backoff.
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_DISCONNECTED));
    state.relevant_airport = "EKCH";

    EXPECT_FALSE(svc->IsBackingOff());
    EXPECT_CALL(*mockImpl, Connect()).Times(1);
    svc->OnTimer(1);
}

// ---------------------------------------------------------------------------
// GetStats / ShouldSend accessors
// ---------------------------------------------------------------------------

TEST_F(WebSocketServiceOnTimerTest, ShouldSend_WhenDisconnected_ReturnsFalse) {
    EXPECT_FALSE(svc->ShouldSend());
}

TEST_F(WebSocketServiceOnTimerTest, ShouldSend_WhenConnectedButSlave_ReturnsFalse) {
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    svc->SetSessionState(STATE_SLAVE);
    EXPECT_FALSE(svc->ShouldSend());
}

TEST_F(WebSocketServiceOnTimerTest, ShouldSend_WhenConnectedAndMaster_ReturnsTrue) {
    ON_CALL(*mockImpl, GetStatus()).WillByDefault(Return(WEBSOCKET_STATUS_CONNECTED));
    svc->SetSessionState(STATE_MASTER);
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

// ---------------------------------------------------------------------------
// ClientState enum
// ---------------------------------------------------------------------------

TEST(ClientStateTest, Values_AreDistinct) {
    using namespace FlightStrips::websocket;
    EXPECT_NE(STATE_UNKNOWN, STATE_SLAVE);
    EXPECT_NE(STATE_SLAVE,   STATE_MASTER);
    EXPECT_NE(STATE_UNKNOWN, STATE_MASTER);
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

TEST(EventTypeTest, LoginEvent_SerializesCorrectType) {
    LoginEvent e("EKCH", "OBS", "GND", "EK_GND", 150);
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"], EVENT_LOGIN_NAME);
}

TEST(EventTypeTest, LoginEvent_SerializesAllFields) {
    LoginEvent e("EKCH", "OBS", "GND", "EK_GND", 150);
    const nlohmann::json j = e;
    EXPECT_EQ(j["airport"],    "EKCH");
    EXPECT_EQ(j["connection"], "OBS");
    EXPECT_EQ(j["position"],   "GND");
    EXPECT_EQ(j["callsign"],   "EK_GND");
    EXPECT_EQ(j["range"],      150);
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
    CoordinationReceivedEvent e("EKS007", "EKCH_APP");
    const nlohmann::json j = e;
    EXPECT_EQ(j["type"],                EVENT_COORDINATION_RECEIVED_NAME);
    EXPECT_EQ(j["callsign"],            "EKS007");
    EXPECT_EQ(j["controller_callsign"], "EKCH_APP");
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
        "stand": "B5"
    })");
    const auto s = j.get<BackendSyncStrip>();
    EXPECT_EQ(s.callsign,        "EKS010");
    EXPECT_EQ(s.assigned_squawk, "2201");
    EXPECT_EQ(s.cleared,         true);
    EXPECT_EQ(s.ground_state,    "PUSH");
    EXPECT_EQ(s.stand,           "B5");
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
