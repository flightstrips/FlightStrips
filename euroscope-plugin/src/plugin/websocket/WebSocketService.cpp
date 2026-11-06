#include "WebSocketService.h"

#include <random>

#include "ExceptionHandling.h"
#include "Events.h"
#include "Logger.hpp"
#include "network/LocalIpAddress.h"
#include "version.h"
#include "websocket/generated/proto/euroscope.pb.h"

namespace FlightStrips::websocket {
    bool WebSocketService::SendHoldReplay(const HoldEvent& event) {
        if (!IsConnected() || m_plugin->GetConnectionState().observer) return false;
        for (const auto& [id, member] : memberships_.members) {
            if (!active_membership_.empty() && id != active_membership_) continue;
            if ((member.role == STATE_MASTER || member.role == STATE_SLAVE) && member.inventory.contains(event.callsign)) {
                // Reconnect replay is source state, not completion of an SDK command.
                SendBytes(protobuf::Serialize(event));
                return true;
            }
        }
        return false;
    }

    namespace {
        using Wire=protobuf::wire::Envelope;
        Wire::EventCase ExpectedResult(const Wire& event) {
            switch(event.event_case()) {
            case Wire::kGenerateSquawk:return Wire::kAssignedSquawk;
            case Wire::kAssignedSquawk:case Wire::kRequestedAltitude:case Wire::kClearedAltitude:case Wire::kGroundState:
            case Wire::kClearedFlag:case Wire::kHeading:case Wire::kStand:case Wire::kCommunicationType:case Wire::kHold:return event.event_case();
            default:return Wire::EVENT_NOT_SET;
            }
        }
        std::string ResultCallsign(const Wire& event) {
            switch(event.event_case()) {
            case Wire::kGenerateSquawk:return event.generate_squawk().callsign();case Wire::kAssignedSquawk:return event.assigned_squawk().callsign();
            case Wire::kRequestedAltitude:return event.requested_altitude().callsign();case Wire::kClearedAltitude:return event.cleared_altitude().callsign();
            case Wire::kGroundState:return event.ground_state().callsign();case Wire::kClearedFlag:return event.cleared_flag().callsign();
            case Wire::kHeading:return event.heading().callsign();case Wire::kStand:return event.stand().callsign();
            case Wire::kCommunicationType:return event.communication_type().callsign();case Wire::kHold:return event.hold().callsign();default:return {};
            }
        }
    }
    bool WebSocketService::ExecutionCurrent(const std::string& id) const {
        const auto found=pending_executions_.find(id);if(found==pending_executions_.end() || !IsConnected() || m_plugin->GetConnectionState().observer) return false;
        const auto& execution=found->second;
        if(std::chrono::steady_clock::now()>=execution.expires) return false;
        const auto member=memberships_.members.find(execution.context.membership_id());
        if(member==memberships_.members.end() || member->second.role==STATE_OBSERVER || member->second.role==STATE_UNKNOWN) return false;
        auto current=member->second.context, captured=execution.context;
        if(execution.origin==protobuf::wire::COMMAND_ORIGIN_HUMAN) {current.set_master_epoch(0);captured.set_master_epoch(0);}
        return current.SerializeAsString()==captured.SerializeAsString();
    }
    bool WebSocketService::ExecutionReady(const std::string& id) const {
        if(!ExecutionCurrent(id)) return false;
        const auto& desired=pending_executions_.at(id);
        for(const auto& [other,pending]:pending_executions_) if(other!=id && pending.started && pending.callsign==desired.callsign && pending.response==desired.response && ExecutionCurrent(other)) return false;
        return true;
    }
    bool WebSocketService::ExecuteDeferred(const std::string& id,const std::function<void()>& action) {
        if(!ExecutionReady(id)) return false;
        auto& execution=pending_executions_.at(id);execution.started=true;
        const auto oldMember=active_membership_,oldExecution=active_execution_;const auto oldOrigin=active_origin_;
        active_membership_=execution.context.membership_id();active_execution_=id;active_origin_=execution.origin;
        try {action();} catch(...) {active_membership_=oldMember;active_execution_=oldExecution;active_origin_=oldOrigin;throw;}
        active_membership_=oldMember;active_execution_=oldExecution;active_origin_=oldOrigin;return true;
    }
    void WebSocketService::DeferSquawk(PendingSquawkQueue& queue,const std::string& callsign) {
        const auto id=active_execution_;
        const auto found=pending_executions_.find(id);
        if(found==pending_executions_.end() || found->second.callsign!=callsign || found->second.response!=Wire::kAssignedSquawk || !ExecutionCurrent(id)) return;
        const auto weak=weak_from_this();
        queue.Add(callsign,[weak,id]{const auto socket=weak.lock();return socket && socket->ExecutionCurrent(id);},
            [weak,id]{const auto socket=weak.lock();return socket && socket->ExecutionReady(id);},
            [weak,id](const std::function<void()>& action){const auto socket=weak.lock();return socket && socket->ExecuteDeferred(id,action);});
    }
    bool WebSocketService::SendCommandResult(const Event& event) {
        if(pending_executions_.empty()) return false;
        Wire result;if(!result.ParseFromString(protobuf::Serialize(event))) return false;
        const auto callsign=ResultCallsign(result);if(callsign.empty()) return false;
        std::string selected;bool matched=false;
        for(auto it=pending_executions_.begin();it!=pending_executions_.end();) {
            auto& pending=it->second;
            const bool candidate=pending.started && pending.callsign==callsign && pending.response==result.event_case() && (pending.expected.empty() || pending.expected==result.SerializeAsString());
            if(!ExecutionCurrent(it->first)) {matched=matched||candidate;it=pending_executions_.erase(it);continue;}
            if(candidate && (active_execution_.empty() || active_execution_==it->first)) {matched=true;if(!selected.empty()) return true;selected=it->first;}
            ++it;
        }
        if(selected.empty()) return matched;
        const auto pending=pending_executions_.at(selected);pending_executions_.erase(selected);
        *result.mutable_context()=pending.context;result.set_command_origin(pending.origin);result.set_execution_id(selected);
        ++tx;webSocket->Send(result.SerializeAsString());
        // The exact business result belongs to its initiating member. The
        // physical SDK fact is also an ordinary observation for every other
        // applicable membership where this instance is the current master.
        for(const auto& [memberId,member]:memberships_.members) {
            if(memberId==pending.context.membership_id() || member.role!=STATE_MASTER || !member.inventory.contains(callsign)) continue;
            auto observation=result;*observation.mutable_context()=member.context;observation.clear_command_origin();observation.clear_execution_id();
            ++tx;webSocket->Send(observation.SerializeAsString());
        }
        return true;
    }

    void WebSocketService::ForgetAircraft(const std::string& callsign) {
        source_aircraft_.erase(callsign);source_runways_.erase(callsign);
        for(auto& [id,m]:memberships_.members) {m.inventory.erase(callsign);m.excluded.insert(callsign);}
    }
    void WebSocketService::RememberAircraft(const std::string& callsign,const std::string& origin,const std::string& destination,double latitude,double longitude,bool hasFp) {
        auto& fact=source_aircraft_[callsign];fact.set_callsign(callsign);fact.set_origin(origin);fact.set_destination(destination);fact.set_has_fp(hasFp);
        if(std::isfinite(latitude) && std::isfinite(longitude)) {fact.mutable_position()->set_lat(latitude);fact.mutable_position()->set_lon(longitude);}
        for(auto& [id,m]:memberships_.members) {
            if(m.role==STATE_MASTER) continue;
            if(m.Includes(fact)) {m.inventory.insert(callsign);m.excluded.erase(callsign);}
            else {m.inventory.erase(callsign);m.excluded.insert(callsign);}
        }
    }
    std::vector<protobuf::wire::MembershipContext> WebSocketService::CaptureObservationContexts(const std::string& callsign) const {
        std::vector<protobuf::wire::MembershipContext> contexts;
        for(const auto& [id,m]:memberships_.members) if(m.role==STATE_MASTER && m.inventory.contains(callsign)) contexts.push_back(m.context);
        return contexts;
    }
    void WebSocketService::SendObservation(const Event& event,const protobuf::wire::MembershipContext& context) {
        const auto it=memberships_.members.find(context.membership_id());
        if(!IsConnected() || it==memberships_.members.end() || it->second.role!=STATE_MASTER ||
            it->second.context.SerializeAsString()!=context.SerializeAsString()) return;
        ++tx;webSocket->Send(protobuf::Serialize(event,context));
    }
    void WebSocketService::ForEachMembership(const std::function<void(const std::string&)>& action) {
        const auto previous=active_membership_;
        try {for(const auto& [id,m]:memberships_.members) {active_membership_=id;action(m.context.airport());}}
        catch(...) {active_membership_=previous;throw;}
        active_membership_=previous;
    }
    std::string WebSocketService::CurrentMembershipId() const {
        if (!active_membership_.empty()) return active_membership_;
        const auto& airport=m_plugin->GetConnectionState().relevant_airport;
        for(const auto& [id,m]:memberships_.members) if(m.context.airport()==airport) return id;
        if(memberships_.members.size()==1) return memberships_.members.begin()->first;
        return {};
    }
    std::string WebSocketService::CurrentAirport() const {
        const auto found=memberships_.members.find(active_membership_);
        return found==memberships_.members.end()?m_plugin->GetConnectionState().relevant_airport:found->second.context.airport();
    }
    protobuf::wire::MembershipContext WebSocketService::CaptureContext() const {
        if (const auto it=memberships_.members.find(active_membership_);it!=memberships_.members.end()) return it->second.context;
        protobuf::wire::MembershipContext context;context.set_instance_id(instance_id_);context.set_transport_generation(transport_generation_);return context;
    }
    void WebSocketService::RememberRunways(const std::string& callsign,const std::string& departure,const std::string& arrival) {
        source_runways_[callsign]={departure,arrival};
    }
    void WebSocketService::ProcessServerMessage(const std::string& bytes) {
        protobuf::wire::Envelope envelope;
        if(!protobuf::ParseEnvelope(bytes,envelope)) return;
        if(envelope.has_control()) {
            const auto& c=envelope.context();
            if(c.instance_id()!=instance_id_ || c.transport_generation()==0 || !c.membership_id().empty()) return;
            if(transport_generation_!=0 && transport_generation_!=c.transport_generation()) return;
            transport_generation_=c.transport_generation();return;
        }
        const auto& state=m_plugin->GetConnectionState();
        const bool accepted=memberships_.Accept(envelope,instance_id_,transport_generation_,GetLoginSessionName(state));
        if(envelope.has_membership_removed() && memberships_.retired.contains(envelope.context().membership_id())) m_plugin->RemoveMembershipAirport(envelope.context().membership_id());
        if(!accepted) return;
        if(envelope.has_session_info()) {
            auto& member=memberships_.members.at(envelope.context().membership_id());
            for(const auto& [callsign,fact]:source_aircraft_) if(member.Includes(fact)) member.inventory.insert(callsign);
        }
        if(envelope.has_backend_sync()) m_plugin->SetMembershipAirport(envelope.context().membership_id(),envelope.context().airport(),envelope.backend_sync().latitude(),envelope.backend_sync().longitude());
        if(!envelope.execution_id().empty() && ExpectedResult(envelope)!=Wire::EVENT_NOT_SET) {
            for(auto it=pending_executions_.begin();it!=pending_executions_.end();) {if(!ExecutionCurrent(it->first)) it=pending_executions_.erase(it);else ++it;}
            if(pending_executions_.size()>=256) return;
            auto expected=envelope;expected.clear_context();expected.clear_command_origin();expected.clear_execution_id();
            pending_executions_[envelope.execution_id()]={envelope.context(),envelope.command_origin(),ResultCallsign(envelope),ExpectedResult(envelope),std::chrono::steady_clock::now()+(envelope.has_generate_squawk()?std::chrono::seconds(120):std::chrono::seconds(30)),!envelope.has_generate_squawk(),envelope.has_generate_squawk()?"":expected.SerializeAsString()};
        }
        active_membership_=envelope.context().membership_id();
        active_origin_=envelope.command_origin();active_execution_=envelope.execution_id();
        // All consumers, including guidance stores, run under the same validated
        // membership on the SDK callback thread, never the websocket thread.
        try { m_messageHandlers->OnMessages({bytes}); }
        catch (...) { active_membership_.clear();active_execution_.clear();active_origin_=protobuf::wire::COMMAND_ORIGIN_UNSPECIFIED;throw; }
        active_membership_.clear();active_execution_.clear();active_origin_=protobuf::wire::COMMAND_ORIGIN_UNSPECIFIED;
    }
    void WebSocketService::SendCaptured(const Event& event,const protobuf::wire::MembershipContext& context) {
        const auto it=memberships_.members.find(context.membership_id());
        if(!IsConnected() || it==memberships_.members.end()) return;
        auto expected=it->second.context, captured=context;expected.set_master_epoch(0);captured.set_master_epoch(0);
        if(expected.SerializeAsString()!=captured.SerializeAsString()) return;
        ++tx;webSocket->Send(protobuf::Serialize(event,context));
    }
    void WebSocketService::SendBytes(const std::string& bytes) {
        protobuf::wire::Envelope source;
        if(!source.ParseFromString(bytes)) return;
        if(source.has_token()) { source.mutable_token()->set_instance_id(instance_id_);++tx;webSocket->Send(source.SerializeAsString());return; }
        if(source.has_login() || transport_generation_==0) return;
        auto send=[&](AirportMembership& member) {
            auto event=source;
            auto prepareStrip=[&](protobuf::wire::Strip* strip) {
                if(const auto it=source_runways_.find(strip->callsign());it!=source_runways_.end()) strip->set_runway(strip->destination()==member.context.airport()?it->second.second:it->second.first);
                if(strip->destination()!=member.context.airport()) {strip->clear_star();strip->clear_eldt();}
            };
            if(event.has_sync()) {
                member.inventory.clear();member.excluded.clear();
                auto* strips=event.mutable_sync()->mutable_strips();
                for(int i=strips->size()-1;i>=0;--i) {
                    auto* strip=strips->Mutable(i);
                    if(!member.Includes(*strip)) {member.excluded.insert(strip->callsign());strips->DeleteSubrange(i,1);continue;}
                    prepareStrip(strip);member.inventory.insert(strip->callsign());
                }
            }
            if(event.has_strip_update()) {
                auto* strip=event.mutable_strip_update()->mutable_strip();
                if(!member.Includes(*strip)) {
                    member.excluded.insert(strip->callsign());
                    // Publish the transition once to the previous airport so
                    // its existing handler can apply admission/removal policy.
                    if(member.inventory.erase(strip->callsign())==0) return;
                } else {member.inventory.insert(strip->callsign());member.excluded.erase(strip->callsign());}
                prepareStrip(strip);
            }
            *event.mutable_context()=member.context;
            if(!active_execution_.empty()) {event.set_command_origin(active_origin_);event.set_execution_id(active_execution_);}
            ++tx;webSocket->Send(event.SerializeAsString());
        };

        std::string callsign;
        // One field name across typed aircraft events. Keep routing explicit for
        // the lite runtime, which deliberately has no protobuf reflection.
        switch(source.event_case()) {
#define AIRCRAFT_CASE(Name,accessor) case protobuf::wire::Envelope::Name:callsign=source.accessor().callsign();break;
        AIRCRAFT_CASE(kAssignedSquawk,assigned_squawk) AIRCRAFT_CASE(kSquawk,squawk)
        AIRCRAFT_CASE(kRequestedAltitude,requested_altitude) AIRCRAFT_CASE(kClearedAltitude,cleared_altitude)
        AIRCRAFT_CASE(kCommunicationType,communication_type) AIRCRAFT_CASE(kGroundState,ground_state)
        AIRCRAFT_CASE(kClearedFlag,cleared_flag) AIRCRAFT_CASE(kAircraftPositionUpdate,aircraft_position_update)
        AIRCRAFT_CASE(kHeading,heading) AIRCRAFT_CASE(kAircraftDisconnect,aircraft_disconnect)
        AIRCRAFT_CASE(kStand,stand) AIRCRAFT_CASE(kTrackingControllerChanged,tracking_controller_changed)
        AIRCRAFT_CASE(kHold,hold) AIRCRAFT_CASE(kFsScratchPad,fs_scratch_pad)
        AIRCRAFT_CASE(kCdmTobtUpdate,cdm_tobt_update) AIRCRAFT_CASE(kCdmAsrtToggle,cdm_asrt_toggle)
        AIRCRAFT_CASE(kCdmTsacUpdate,cdm_tsac_update) AIRCRAFT_CASE(kCdmDeiceUpdate,cdm_deice_update)
        AIRCRAFT_CASE(kCdmManualCtot,cdm_manual_ctot) AIRCRAFT_CASE(kCdmCtotRemove,cdm_ctot_remove) AIRCRAFT_CASE(kCdmReady,cdm_ready)
        AIRCRAFT_CASE(kCoordinationReceived,coordination_received)
        AIRCRAFT_CASE(kIssuePdcClearance,issue_pdc_clearance) AIRCRAFT_CASE(kPdcRevertToVoice,pdc_revert_to_voice)
        case protobuf::wire::Envelope::kAmanRouteFact:callsign=source.aman_route_fact().data().callsign();break;
        default:break;
#undef AIRCRAFT_CASE
        }
        if(!active_membership_.empty()) {
            if(auto it=memberships_.members.find(active_membership_);it!=memberships_.members.end() && (it->second.role==STATE_MASTER || it->second.role==STATE_SLAVE || source.has_runway())) {
                if(!callsign.empty() && !it->second.inventory.contains(callsign) && active_execution_.empty()) return;
                send(it->second);
            }
            return;
        }
        const bool human=source.has_issue_pdc_clearance() || source.has_pdc_revert_to_voice() || source.has_fs_scratch_pad() || source.has_cdm_tobt_update() || source.has_cdm_asrt_toggle() || source.has_cdm_tsac_update() || source.has_cdm_deice_update() || source.has_cdm_manual_ctot() || source.has_cdm_ctot_remove() || source.has_cdm_ready();
        AirportMembership* own=nullptr;
        if(human) {
            if(auto selected=memberships_.members.find(CurrentMembershipId());selected!=memberships_.members.end() && selected->second.role!=STATE_OBSERVER && selected->second.inventory.contains(callsign)) {send(selected->second);return;}
            for(auto& [id,m]:memberships_.members) if(m.role!=STATE_OBSERVER && m.inventory.contains(callsign)) {if(own) return;own=&m;}
            if(own) send(*own);return;
        }
        for(auto& [id,member]:memberships_.members) {
            if(member.role!=STATE_MASTER && !((source.has_hold() || source.has_aman_route_fact() || source.has_coordination_received()) && member.role==STATE_SLAVE)) continue;
            if(!callsign.empty() && !member.inventory.contains(callsign)) continue;
            send(member);
            if(source.has_aircraft_disconnect()) {member.inventory.erase(callsign);member.excluded.insert(callsign);}
        }
        if(source.has_aircraft_disconnect()) {
            source_aircraft_.erase(callsign);source_runways_.erase(callsign);
            for(auto& [id,m]:memberships_.members) {m.inventory.erase(callsign);m.excluded.insert(callsign);}
        }
    }
    namespace {
        std::string RandomIdentity() {
            std::random_device random; constexpr char hex[]="0123456789abcdef";
            std::string id; for (int i=0;i<16;++i) { const auto b=random()&255; id+=hex[b>>4];id+=hex[b&15]; } return id;
        }
    }

    WebSocketService::WebSocketService(std::string baseUrl,
                                       const bool apiEnabled,
                                       const std::shared_ptr<authentication::IAuthenticationService> &
                                       authentication_service,
                                       const std::shared_ptr<IFlightStripsPlugin> &plugin,
                                       const std::shared_ptr<handlers::ConnectionEventHandlers> & event_handlers,
                                       const std::shared_ptr<handlers::MessageHandlers>& message_handlers) :
                                                         m_base_url(baseUrl),
                                                         m_authentication_service(authentication_service),
                                                         m_plugin(plugin),
                                                         m_connection_handlers(event_handlers),
                                                         m_messageHandlers(message_handlers),
                                                         webSocket(std::make_unique<WebSocket>(
                                                             std::move(baseUrl),
                                                             [this](const std::string &message) {
                                                                 this->OnMessage(message);
                                                             }, [this] { this->OnConnected(); })),
                                                         enabled(apiEnabled),
                                                         local_ip_(network::GetLocalPrivateIPv4().value_or("")) {
        instance_id_=RandomIdentity();
        if (enabled) {
            InitializeOnlineState();
        }
    }

    WebSocketService::WebSocketService(const std::shared_ptr<authentication::IAuthenticationService> &authentication_service,
                                       const std::shared_ptr<IFlightStripsPlugin> &plugin,
                                       const std::shared_ptr<handlers::ConnectionEventHandlers> &event_handlers,
                                       const std::shared_ptr<handlers::MessageHandlers> &message_handlers,
                                       std::unique_ptr<WebSocket> ws,
                                       const bool enabled,
                                       std::string localIp) :
                                                         m_authentication_service(authentication_service),
                                                         m_plugin(plugin),
                                                         m_connection_handlers(event_handlers),
                                                         m_messageHandlers(message_handlers),
                                                         webSocket(std::move(ws)),
                                                         enabled(enabled),
                                                         local_ip_(std::move(localIp)) {
        instance_id_=RandomIdentity();
        if (enabled) {
            InitializeOnlineState();
        }
    }

    WebSocketService::~WebSocketService() {
    }

    void WebSocketService::OnTimer(int time) {
        if (!enabled) return;
        ProcessConnected();
        if (IsConnected()) {
            std::optional<std::string> token;
            { std::lock_guard lock(message_mutex_);
              if (token_update_pending_) { token = latest_token_; token_update_pending_ = false; } }
            if (token) SendEvent(TokenEvent(*token, PLUGIN_VERSION));
        }
        const auto &state = m_plugin->GetConnectionState();
        UpdatePlaybackSession(state.connection_type);
        const auto now = std::chrono::steady_clock::now();
        UpdateOnlineState(state.connection_type != CONNECTION_TYPE_NO, now);

        const bool freq_ok = !state.primary_frequency.empty() && state.primary_frequency != "199.998";
        const bool airport_ok = !state.relevant_airport.empty();
        const bool conn_ok = state.connection_type == CONNECTION_TYPE_SWEATBOX || state.connection_type ==
                             CONNECTION_TYPE_DIRECT || state.connection_type == CONNECTION_TYPE_PLAYBACK;
        const bool auth_ok = m_authentication_service->GetAuthenticationState() == authentication::AUTHENTICATED
                          || m_authentication_service->GetAuthenticationState() == authentication::REFRESH;
        const bool should_connect = conn_ok && auth_ok;

        // Clear a different exercise's guidance even while waiting to connect.
        if (conn_ok) {
            m_connection_handlers->OnSessionChanged(GetLoginSessionName(state));
        }

        if (!should_connect && IsConnected()) {
            Logger::Warning("Disconnecting from server — reason: freq_ok={} (freq='{}') airport_ok={} (airport='{}') conn_ok={} (type={}) auth_ok={}",
                freq_ok, state.primary_frequency,
                airport_ok, state.relevant_airport,
                conn_ok, static_cast<int>(state.connection_type),
                auth_ok);
        }

        if (should_connect && (webSocket->GetStatus() == WEBSOCKET_STATUS_DISCONNECTED || webSocket->GetStatus() ==
                               WEBSOCKET_STATUS_FAILED)) {
            if (!connect_after_.has_value()) {
                pending_connect_ = true;
                if (webSocket->GetStatus() == WEBSOCKET_STATUS_FAILED) {
                    const auto delay_ms = BACKOFF_MS[std::min(fail_count_, static_cast<int>(BACKOFF_MS.size()) - 1)];
                    fail_count_++;
                    connect_after_ = now + std::chrono::milliseconds(delay_ms);
                    Logger::Info("Connect failed (attempt {}), retrying in {}ms", fail_count_, delay_ms);
                    return;
                }

                if (const auto connect_not_before = GetConnectNotBefore(now); connect_not_before.has_value()) {
                    connect_after_ = connect_not_before;
                    const auto delay_seconds = std::chrono::duration_cast<std::chrono::seconds>(
                        connect_not_before.value() - now).count();
                    Logger::Info("Waiting {}s before connecting to server", delay_seconds);
                    return;
                }
            }
            if (connect_after_.has_value() && now < connect_after_.value()) {
                return;
            }
            connect_after_.reset();
            primary = state.primary_frequency;
            Logger::Info("Trying to connect to server: {}", m_base_url);
            webSocket->Connect();
            return;
        }

        if (!should_connect && IsConnected()) {
            webSocket->Disconnect();
            std::lock_guard lock(message_mutex_);
            messages_.clear();

            for(const auto& [id,m]:memberships_.members) m_plugin->RemoveMembershipAirport(id);
            pending_executions_.clear();memberships_.Reset(); active_membership_.clear(); transport_generation_=0;
            observer = false;
            connect_after_.reset();
            pending_connect_ = false;
            fail_count_ = 0;
            return;
        }

        if (!should_connect || !IsConnected()) {
            if (!should_connect) {
                connect_after_.reset();
                pending_connect_ = false;
                fail_count_ = 0;
            }
            return;
        }

        if (!session_name.empty() && session_name != GetLoginSessionName(state)) {
			m_connection_handlers->OnSessionChanged(GetLoginSessionName(state));
            Logger::Info("Session mode changed: '{}' -> '{}', reconnecting", session_name, GetLoginSessionName(state));
            Reconnect();
            return;
        }

        if (last_control_at_.has_value() && (observer != state.observer || last_control_callsign_!=state.callsign)) {
            for(const auto& [id,m]:memberships_.members) {memberships_.retired.insert(id);m_plugin->RemoveMembershipAirport(id);}
            memberships_.members.clear();active_membership_.clear();
        }
        if (primary != state.primary_frequency || observer!=state.observer || last_control_callsign_!=state.callsign ||
            last_control_airport_!=state.relevant_airport || last_control_range_!=state.range ||
            !last_control_at_.has_value() || now-*last_control_at_>=std::chrono::seconds(10)) SendLoginEvent();

        std::vector<std::string> messages;
        {
            std::lock_guard lock(message_mutex_);
            if (messages_.empty()) return;
            Logger::Info("Got messages {} from server", messages_.size());
            messages = std::move(messages_);
        }

        ++dispatch_batch_;
        for (const auto& bytes:messages) ProcessServerMessage(bytes);
    }

    void WebSocketService::OnTokenUpdate(const std::string &token) {
        // Authentication completes on its worker thread. Membership routing,
        // SDK connection state, and transmission counters belong to OnTimer.
        std::lock_guard lock(message_mutex_);
        latest_token_ = token;
        token_update_pending_ = true;
    }

    bool WebSocketService::IsConnected() const {
        return webSocket->GetStatus() == WEBSOCKET_STATUS_CONNECTED && !connected_pending_.load();
    }

    bool WebSocketService::IsPendingConnect() const {
        return pending_connect_ || webSocket->GetStatus() == WEBSOCKET_STATUS_CONNECTING;
    }

    bool WebSocketService::IsBackingOff() const {
        return fail_count_ > 0 && connect_after_.has_value();
    }

    bool WebSocketService::ShouldSend() const {
        if (!IsConnected() || m_plugin->GetConnectionState().observer) return false;
        if (!active_membership_.empty()) { const auto it=memberships_.members.find(active_membership_);return it!=memberships_.members.end() && it->second.role==STATE_MASTER; }
        for (const auto& [id,m]:memberships_.members) if(m.role==STATE_MASTER) return true;
        return false;
    }

    bool WebSocketService::ShouldSendTrackedAircraft(const bool trackingControllerIsMe) const {
        if (!trackingControllerIsMe || !IsConnected() || m_plugin->GetConnectionState().observer) return false;
        if (!active_membership_.empty()) { const auto it=memberships_.members.find(active_membership_);return it!=memberships_.members.end() && (it->second.role==STATE_MASTER || it->second.role==STATE_SLAVE); }
        for(const auto& [id,m]:memberships_.members) if(m.role==STATE_MASTER || m.role==STATE_SLAVE) return true;
        return false;
    }

    bool WebSocketService::ShouldProcessServerMessageType(const std::string &type) const {
        const auto it=memberships_.members.find(active_membership_);
        if (!(m_plugin->GetConnectionState().observer || (it!=memberships_.members.end() && it->second.role==STATE_OBSERVER))) {
            return true;
        }

        return type == EVENT_SESSION_INFO_NAME || type == EVENT_MASTER_INFO_NAME || type == EVENT_RUNWAY_MISMATCH_ALERT_NAME || type == "aman_gain_loss";
    }

    void WebSocketService::Reconnect() {
        connect_after_.reset();
        pending_connect_ = false;
        fail_count_ = 0;
        primary.clear();
        session_name.clear();
        observer = false;

        if (IsConnected()) {
            webSocket->Disconnect();
        }

        std::lock_guard lock(message_mutex_);
        messages_.clear();

            for(const auto& [id,m]:memberships_.members) m_plugin->RemoveMembershipAirport(id);
            pending_executions_.clear();memberships_.Reset(); active_membership_.clear(); transport_generation_=0;

    }

    void WebSocketService::SetSessionState(const ClientState state) {

        if (auto it=memberships_.members.find(active_membership_);it!=memberships_.members.end()) it->second.role=state;
    }

    void WebSocketService::SetMasterCallsign(const std::string& callsign) {
        std::lock_guard lock(message_mutex_);

        if (auto it=memberships_.members.find(CurrentMembershipId());it!=memberships_.members.end()) it->second.masterCallsign=callsign;
    }

    bool WebSocketService::CanSendEventType(const EventType type) const {
        if (connected_pending_.load()) return false;
        if (type == EVENT_AIRCRAFT_POSITION_UPDATE) return ShouldSend();
        const auto it=memberships_.members.find(active_membership_);
        if (!(m_plugin->GetConnectionState().observer || (it!=memberships_.members.end() && it->second.role==STATE_OBSERVER))) {
            return true;
        }

        return type == EVENT_TOKEN || type == EVENT_LOGIN || type == EVENT_RUNWAY;
    }

    Stats WebSocketService::GetStats() const {
        std::lock_guard lock(message_mutex_);
        const auto it=memberships_.members.find(CurrentMembershipId());
        return Stats{tx,rx,static_cast<int>(messages_.size()),it==memberships_.members.end()?STATE_UNKNOWN:it->second.role,it==memberships_.members.end()?"":it->second.masterCallsign};
    }

    std::optional<int> WebSocketService::GetDelaySecondsRemaining() const {
        if (!connect_after_.has_value()) return std::nullopt;
        const auto remaining = std::chrono::duration_cast<std::chrono::seconds>(
            connect_after_.value() - std::chrono::steady_clock::now()).count();
        if (remaining <= 0) return std::nullopt;
        return static_cast<int>(remaining);
    }



    void WebSocketService::OnMessage(const std::string &message) {
        exceptions::RunGuarded("WebSocketService::OnMessage", [this, &message] {
            rx++;
            protobuf::wire::Envelope envelope;
            if (!protobuf::ParseEnvelope(message, envelope)) {
                Logger::Warning("Invalid protobuf websocket message ({} bytes)", message.size());
                return;
            }

            std::lock_guard lock(message_mutex_);
            messages_.push_back(message);
        });
    }

    void WebSocketService::OnConnected() {
        connected_pending_.store(true);
    }
    void WebSocketService::ProcessConnected() {
        if (webSocket->GetStatus() != WEBSOCKET_STATUS_CONNECTED || !connected_pending_.exchange(false)) return;
        exceptions::RunGuarded("WebSocketService::OnConnected", [this] {
            SetMasterCallsign("");
            for(const auto& [id,m]:memberships_.members) m_plugin->RemoveMembershipAirport(id);
            pending_executions_.clear();memberships_.Reset(); active_membership_.clear();transport_generation_=0;source_runways_.clear();source_aircraft_.clear();
            std::optional<std::string> connectedToken;
            { std::lock_guard lock(message_mutex_); messages_.clear();
              connectedToken = latest_token_; token_update_pending_ = false; }
            tx = 0;
            rx = 0;
            pending_connect_ = false;
            fail_count_ = 0;
            connect_after_.reset();
            const bool online = m_plugin->GetConnectionState().connection_type != CONNECTION_TYPE_NO;
            was_online_ = online;
            if (online && !online_since_.has_value()) {
                online_since_ = std::chrono::steady_clock::now();
            }
            has_connected_in_online_period_ = true;
            const auto token = TokenEvent(connectedToken ? *connectedToken : m_authentication_service->GetAccessToken(), PLUGIN_VERSION);
            SendEvent(token);
            SendLoginEvent();

            m_connection_handlers->OnOnline();
        });
    }

    void WebSocketService::InitializeOnlineState() {
        const bool online = m_plugin->GetConnectionState().connection_type != CONNECTION_TYPE_NO;
        was_online_ = online;
        if (!online) {
            return;
        }

        online_since_ = std::chrono::steady_clock::now();
        online_connect_delay_seconds_ = FAST_CONNECT_DELAY_SECONDS;
    }

    void WebSocketService::UpdateOnlineState(const bool online, const std::chrono::steady_clock::time_point now) {
        if (online == was_online_) {
            return;
        }
        if (online && online_since_.has_value()) {
            was_online_ = true;
            return;
        }

        was_online_ = online;
        if (online) {
            online_since_ = now;
            has_connected_in_online_period_ = false;
            online_connect_delay_seconds_ = ONLINE_TRANSITION_CONNECT_DELAY_SECONDS;
            return;
        }

        online_since_.reset();
        has_connected_in_online_period_ = false;
        online_connect_delay_seconds_ = ONLINE_TRANSITION_CONNECT_DELAY_SECONDS;
    }

    std::optional<std::chrono::steady_clock::time_point> WebSocketService::GetConnectNotBefore(
        const std::chrono::steady_clock::time_point now) const {
        if (has_connected_in_online_period_) {
            return now + std::chrono::seconds(FAST_CONNECT_DELAY_SECONDS);
        }
        if (!online_since_.has_value()) {
            return std::nullopt;
        }

        const auto connect_at = online_since_.value() + std::chrono::seconds(online_connect_delay_seconds_);
        if (connect_at <= now) {
            return std::nullopt;
        }

        return connect_at;
    }

    void WebSocketService::UpdatePlaybackSession(const ConnectionType connectionType) {
        if (connectionType != CONNECTION_TYPE_PLAYBACK) {
            playback_session_name_.clear();
            return;
        }
        if (!playback_session_name_.empty()) return;

        // A fresh 128-bit identifier for each playback. Never reset it on a
        // WebSocket reconnect, frequency change, or backend restart.
        std::random_device random;
        std::uniform_int_distribution<int> byte(0, 255);
        constexpr char hex[] = "0123456789ABCDEF";
        std::string name = "PLAYBACK_";
        for (int i = 0; i < 16; ++i) {
            const auto value = byte(random);
            name += hex[value >> 4];
            name += hex[value & 0xF];
        }
        playback_session_name_ = std::move(name);
    }

    std::string WebSocketService::GetLoginSessionName(const ConnectionState& state) const {
        return state.connection_type == CONNECTION_TYPE_PLAYBACK
            ? playback_session_name_ : GetEffectiveSessionName(state);
    }

    void WebSocketService::SendLoginEvent() {
        const auto& state = m_plugin->GetConnectionState();
        UpdatePlaybackSession(state.connection_type);
        primary = state.primary_frequency;
        session_name = GetLoginSessionName(state);
		m_connection_handlers->OnSessionChanged(session_name);
        observer = state.observer;

        protobuf::wire::Envelope envelope;
        auto* control=envelope.mutable_control();
        control->set_environment(session_name);control->set_callsign(state.callsign);
        control->set_primary_frequency(state.primary_frequency);control->set_observer(state.observer);
        control->set_relevant_airport(state.relevant_airport);control->set_range(state.range);control->set_local_ip(local_ip_);
        for (const auto& airport : m_plugin->GetEligibleAirports()) control->add_eligible_airports(airport);
        // EuroScope exposes primary here, not a confirmed complete tuned-radio
        // list. Unknown tuned evidence must remain unknown for MA-04 policy.
        if(transport_generation_!=0) *envelope.mutable_context()=CaptureContext();
        last_control_callsign_=state.callsign;last_control_airport_=state.relevant_airport;last_control_range_=state.range;last_control_at_=std::chrono::steady_clock::now();
        ++tx;webSocket->Send(envelope.SerializeAsString());
    }
}
