#pragma once
#include <array>
#include <chrono>
#include <optional>
#include <string>
#include "Events.h"
#include "ProtoCodec.h"
#include "Logger.hpp"
#include "WebSocket.h"
#include "MembershipState.h"
#include "PendingSquawkQueue.h"
#include "authentication/IAuthenticationService.h"
#include "handlers/AuthenticationEventHandler.h"
#include "handlers/ConnectionEventHandlers.h"
#include "handlers/MessageHandlers.h"
#include "handlers/TimedEventHandler.h"
#include "plugin/IFlightStripsPlugin.h"

namespace FlightStrips::websocket {

    struct Stats {
        int tx = 0;
        int rx = 0;
        int queued = 0;
        ClientState role = STATE_UNKNOWN;
        std::string masterCallsign;
    };

    class WebSocketService : public handlers::TimedEventHandler, public handlers::AuthenticationEventHandler, public std::enable_shared_from_this<WebSocketService> {
    public:
        explicit WebSocketService(std::string baseUrl,
                                  bool apiEnabled,
                                  const std::shared_ptr<authentication::IAuthenticationService> &authentication_service,
                                  const std::shared_ptr<IFlightStripsPlugin> &plugin,
                                  const std::shared_ptr<handlers::ConnectionEventHandlers> &event_handlers,
                                  const std::shared_ptr<handlers::MessageHandlers> &message_handlers);

        ~WebSocketService() override;

        void OnTimer(int time) override;
        void OnTokenUpdate(const std::string &token) override;

        template<typename T> requires std::is_base_of_v<Event, T>
        void SendEvent(const T &event);
        bool IsConnected() const;
        bool IsPendingConnect() const;
        bool IsBackingOff() const;
        bool ShouldSend() const;
        bool ShouldSendTrackedAircraft(bool trackingControllerIsMe) const;
        bool ShouldProcessServerMessageType(const std::string& type) const;
        void Reconnect();
        void SetSessionState(ClientState state);
        void SetMasterCallsign(const std::string& callsign);
        std::string CurrentMembershipId() const;
        std::string CurrentAirport() const;
        uint64_t DispatchBatch() const { return dispatch_batch_; }
        const MembershipState& Memberships() const { return memberships_; }
        void SendCaptured(const Event& event, const protobuf::wire::MembershipContext& context);
        std::vector<protobuf::wire::MembershipContext> CaptureObservationContexts(const std::string& callsign) const;
        void SendObservation(const Event& event,const protobuf::wire::MembershipContext& context);
        protobuf::wire::MembershipContext CaptureContext() const;
        void RememberRunways(const std::string& callsign, const std::string& departure, const std::string& arrival);
        void RememberAircraft(const std::string& callsign,const std::string& origin,const std::string& destination,double latitude,double longitude,bool hasFp=true);
        void SendRadarOnlyIntroduction(const StripUpdateEvent& event);
        bool KnowsAircraft(const std::string& callsign) const {
            if(source_aircraft_.contains(callsign)) return true;
            for(const auto& [id,m]:memberships_.members) if(m.inventory.contains(callsign)) return true;
            return false;
        }
        void ForgetAircraft(const std::string& callsign);
        bool SendHoldReplay(const HoldEvent& event);
        void ProcessServerMessage(const std::string& bytes);
        void ForEachMembership(const std::function<void(const std::string&)>& action);
        void DeferSquawk(PendingSquawkQueue& queue,const std::string& callsign);
        bool SendCommandResult(const Event& event);
        Stats GetStats() const;
        std::optional<int> GetDelaySecondsRemaining() const;

    protected:
        // Test seam: construct with a pre-built WebSocket (e.g. wrapping a mock ImplBase).
        WebSocketService(const std::shared_ptr<authentication::IAuthenticationService> &authentication_service,
                         const std::shared_ptr<IFlightStripsPlugin> &plugin,
                         const std::shared_ptr<handlers::ConnectionEventHandlers> &event_handlers,
                         const std::shared_ptr<handlers::MessageHandlers> &message_handlers,
                         std::unique_ptr<WebSocket> webSocket,
                         bool enabled,
                         std::string localIp = "");

        void OnConnected();
        void ProcessConnected();
        std::optional<std::chrono::steady_clock::time_point> online_since_;

    private:
        std::string m_base_url;
        std::shared_ptr<authentication::IAuthenticationService> m_authentication_service;
        std::shared_ptr<IFlightStripsPlugin> m_plugin;
        std::shared_ptr<handlers::ConnectionEventHandlers> m_connection_handlers;
        std::shared_ptr<handlers::MessageHandlers> m_messageHandlers;
        std::unique_ptr<WebSocket> webSocket;
        std::string primary;
        std::string session_name;
        // Identifies the EuroScope playback, independently of the backend connection.
        std::string playback_session_name_;
        bool observer = false;
        std::string instance_id_;
        uint64_t transport_generation_ = 0;
        MembershipState memberships_;
        std::string active_membership_;
        protobuf::wire::CommandOrigin active_origin_=protobuf::wire::COMMAND_ORIGIN_UNSPECIFIED;
        std::string active_execution_;
        struct PendingExecution {
            protobuf::wire::MembershipContext context;
            protobuf::wire::CommandOrigin origin;
            std::string callsign;
            protobuf::wire::Envelope::EventCase response;
            std::chrono::steady_clock::time_point expires;
            bool started=false;
            std::string expected;
        };
        std::unordered_map<std::string,PendingExecution> pending_executions_;
        bool ExecutionCurrent(const std::string& id) const;
        bool ExecutionReady(const std::string& id) const;
        bool ExecuteDeferred(const std::string& id,const std::function<void()>& action);
        uint64_t dispatch_batch_ = 0;
        std::unordered_map<std::string, std::pair<std::string,std::string>> source_runways_;
        std::unordered_map<std::string,protobuf::wire::Strip> source_aircraft_;
        std::string local_ip_;
        std::string last_control_callsign_, last_control_airport_;
        int last_control_range_ = 0;
        std::optional<std::chrono::steady_clock::time_point> last_control_at_;

        mutable std::mutex message_mutex_;
        std::vector<std::string> messages_ {};
        std::optional<std::string> latest_token_;
        bool token_update_pending_ = false;

        bool enabled;

        static constexpr int FAST_CONNECT_DELAY_SECONDS = 5;
        static constexpr int ONLINE_TRANSITION_CONNECT_DELAY_SECONDS = 30;
        static constexpr std::array<int, 7> BACKOFF_MS = {500, 1000, 2000, 5000, 10000, 15000, 30000};

        bool was_online_ = false;
        bool has_connected_in_online_period_ = false;
        int online_connect_delay_seconds_ = ONLINE_TRANSITION_CONNECT_DELAY_SECONDS;

        std::optional<std::chrono::steady_clock::time_point> connect_after_;
        bool pending_connect_ = false;
        int fail_count_ = 0;

        int tx = 0;
        std::atomic<int> rx = 0;
        std::atomic<bool> connected_pending_ = false;

        void InitializeOnlineState();
        void UpdatePlaybackSession(ConnectionType connectionType);
        std::string GetLoginSessionName(const ConnectionState& state) const;
        void UpdateOnlineState(bool online, std::chrono::steady_clock::time_point now);
        std::optional<std::chrono::steady_clock::time_point> GetConnectNotBefore(std::chrono::steady_clock::time_point now) const;
        bool CanSendEventType(EventType type) const;
        void OnMessage(const std::string &message);
        void SendLoginEvent();
        void SendBytes(const std::string& bytes);
    };
}

template<typename T> requires std::is_base_of_v<Event, T>
void FlightStrips::websocket::WebSocketService::SendEvent(const T &event) {
    if (SendCommandResult(event)) return;
    if (!CanSendEventType(event.type)) {
        Logger::Debug("Suppressing event type {} for current client role", static_cast<int>(event.type));
        return;
    }
    const auto bytes = protobuf::Serialize(event);
    SendBytes(bytes);
    Logger::Debug("Sending protobuf event type {} ({} bytes)", static_cast<int>(event.type), bytes.size());
}

template void FlightStrips::websocket::WebSocketService::SendEvent<AircraftDisconnectEvent>(const AircraftDisconnectEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<AssignedSquawkEvent>(const AssignedSquawkEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<ClearedAltitudeEvent>(const ClearedAltitudeEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<ClearedFlagEvent>(const ClearedFlagEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<CommunicationTypeEvent>(const CommunicationTypeEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<ControllerOfflineEvent>(const ControllerOfflineEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<ControllerOnlineEvent>(const ControllerOnlineEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<GroundStateEvent>(const GroundStateEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<HeadingEvent>(const HeadingEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<PositionEvent>(const PositionEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<RequestedAltitudeEvent>(const RequestedAltitudeEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<RunwayEvent>(const RunwayEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<CdmTobtUpdateEvent>(const CdmTobtUpdateEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<CdmAsrtToggleEvent>(const CdmAsrtToggleEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<CdmDeiceUpdateEvent>(const CdmDeiceUpdateEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<CdmManualCtotEvent>(const CdmManualCtotEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<CdmCtotRemoveEvent>(const CdmCtotRemoveEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<CdmReadyEvent>(const CdmReadyEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<SquawkEvent>(const SquawkEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<StandEvent>(const StandEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<TrackingControllerChangedEvent>(const TrackingControllerChangedEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<StripUpdateEvent>(const StripUpdateEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<SyncEvent>(const SyncEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<CoordinationReceivedEvent>(const CoordinationReceivedEvent & event);
template void FlightStrips::websocket::WebSocketService::SendEvent<AMANRouteFactEvent>(const AMANRouteFactEvent & event);

template void FlightStrips::websocket::WebSocketService::SendEvent<FsScratchPadEvent>(const FsScratchPadEvent & event);
