#include <grpcpp/security/credentials.h>
#include <grpcpp/create_channel.h>
#include "NetworkService.h"

namespace FlightStrips::network {

    void NetworkService::FlightPlanEvent(EuroScopePlugIn::CFlightPlan flightPlan) {
        if (!ShouldSend() || flightPlan.GetSimulated()) return;

        auto full = ConvertToFullData(flightPlan);

        StripData data;
        data.set_callsign(flightPlan.GetCallsign());
        data.mutable_fulldata()->CopyFrom(full);

        ClientStreamMessage message;
        message.set_clientid(position);
        message.mutable_strip_data()->CopyFrom(data);


        plugin->Information(std::string(1, flightPlan.GetFlightPlanData().GetCommunicationType()));
        plugin->Information(message.DebugString());

        reader->AddMessage(message);
    }

    CommunicationType NetworkService::GetCommunicationType(const EuroScopePlugIn::CFlightPlan &flightPlan) {
        const CommunicationType controller = GetCommunicationType(flightPlan.GetControllerAssignedData().GetCommunicationType());

        if (controller == UNASSIGNED) {
            return GetCommunicationType(flightPlan.GetFlightPlanData().GetCommunicationType());
        }

        return controller;
    }

    CommunicationType NetworkService::GetCommunicationType(const char type) {
        switch (type) {
            case 'v':
                return VOICE;
            case 'r':
                return RECEIVE;
            case 't':
                return TEXT;
            case '0':
            default:
                return UNASSIGNED;
        }
    }

    void NetworkService::ControllerFlightPlanDataEvent(EuroScopePlugIn::CFlightPlan flightPlan,
                                                       int dataType) {
        if (!ShouldSend()) return;

        StripData data;
        data.set_callsign(flightPlan.GetCallsign());

        switch (dataType) {
            case EuroScopePlugIn::CTR_DATA_TYPE_SQUAWK: {
                Squawk squawk;
                squawk.set_squawk(std::string(flightPlan.GetControllerAssignedData().GetSquawk()));
                data.mutable_assigned_squawk()->CopyFrom(squawk);
                break;
            }
            case EuroScopePlugIn::CTR_DATA_TYPE_FINAL_ALTITUDE: {
                FinalAltitude final;
                final.set_altitude(flightPlan.GetControllerAssignedData().GetFinalAltitude());
                data.mutable_final_altitude()->CopyFrom(final);
                break;
            }
            case EuroScopePlugIn::CTR_DATA_TYPE_TEMPORARY_ALTITUDE: {
                ClearedAltitude cleared;
                cleared.set_altitude(flightPlan.GetControllerAssignedData().GetClearedAltitude());
                data.mutable_cleared_altitude()->CopyFrom(cleared);
                break;
            }
            case EuroScopePlugIn::CTR_DATA_TYPE_COMMUNICATION_TYPE: {
                data.set_communication_type(GetCommunicationType(flightPlan));
                break;
            }
            case EuroScopePlugIn::CTR_DATA_TYPE_GROUND_STATE: {
                GroundStateUpdate groundState;
                groundState.set_state(GetGroundState(flightPlan));
                data.mutable_ground_state()->CopyFrom(groundState);
            }
            case EuroScopePlugIn::CTR_DATA_TYPE_CLEARENCE_FLAG: {
                ClearedFlag flag;
                flag.set_cleared(flightPlan.GetClearenceFlag());
                data.mutable_cleared()->CopyFrom(flag);
                break;
            }
            default:
                return;
        }

        ClientStreamMessage message;
        message.set_clientid(position);
        message.mutable_strip_data()->CopyFrom(data);

        reader->AddMessage(message);
    }

    void NetworkService::FlightPlanDisconnectEvent(EuroScopePlugIn::CFlightPlan flightPlan) {
        if (!ShouldSend()) return;

        Disconnect disconnect;
        StripData data;
        data.set_callsign(flightPlan.GetCallsign());
        data.mutable_disconnect()->CopyFrom(disconnect);
        ClientStreamMessage message;
        message.set_clientid(position);

        reader->AddMessage(message);
    }

    void NetworkService::SquawkUpdateEvent(std::string callsign, std::string squawk) {
        if (!ShouldSend()) return;

        Squawk s;
        s.set_squawk(squawk);

        StripData data;
        data.set_callsign(callsign);
        data.mutable_set_squawk()->CopyFrom(s);

        ClientStreamMessage message;
        message.set_clientid(position);
        message.mutable_strip_data()->CopyFrom(data);

        reader->AddMessage(message);
    }

    void NetworkService::ControllerPositionUpdateEvent(EuroScopePlugIn::CController controller) {
        auto isMe = plugin->ControllerIsMe(controller, plugin->ControllerMyself());

        if (isMe) {
            frequency = controller.GetPrimaryFrequency();
            position = std::string(controller.GetCallsign());
        }


        if (!ShouldSend()) return;

        ControllerUpdate update;
        update.set_frequency(std::format("{:.3f}", controller.GetPrimaryFrequency()));
        update.set_callsign(std::string(controller.GetCallsign()));
        update.set_connection_status(CONNECTED);

        ClientStreamMessage message;
        message.set_clientid(position);
        message.mutable_controller_update()->CopyFrom(update);

        reader->AddMessage(message);
    }

    void NetworkService::ControllerDisconnectEvent(EuroScopePlugIn::CController controller) {
        if (!ShouldSend()) return;

        ControllerUpdate update;
        update.set_callsign(std::string(controller.GetCallsign()));
        update.set_connection_status(DISCONNECTED);

        ClientStreamMessage message;
        message.set_clientid(position);
        message.mutable_controller_update()->CopyFrom(update);

        reader->AddMessage(message);
    }

    void NetworkService::OnTimer(int time) {
        int currentConnection = plugin->GetConnectionType();

        if (currentConnection != connectionStatus) {
            if (Online(currentConnection)) {
                onlineTime = time;
            } else {
                onlineTime = 0;
                frequency = DEFAULT_FREQUENCY;
                initialized = false;
                isMaster = false;
                if (reader) {
                    reader->TryCancel();
                    // TODO this MUST be moved as it will block the UI thread.
                    reader->WaitForOnDone();
                    reader.reset();
                }
            }

            connectionStatus = currentConnection;
        }

        if (!initialized && Online(connectionStatus) && time - onlineTime > DELAY_IN_SECONDS && frequency != DEFAULT_FREQUENCY) {
            initialized = true;
            plugin->Information("Connecting");

            reader = client.StartConnection([this](const ServerStreamMessage& message) {
                this->OnNetworkMessage(message);
            });

            Session session;
            session.set_airport("EKCH");

            if (connectionStatus == EuroScopePlugIn::CONNECTION_TYPE_DIRECT) {
                session.set_session("LIVE");
            } else if (connectionStatus == EuroScopePlugIn::CONNECTION_TYPE_SWEATBOX) {
                session.set_session("SWEATBOX");
            } else {
                auto now = std::chrono::system_clock::now();
                time_t time_t =  std::chrono::system_clock::to_time_t( now );
                session.set_session(std::format("PLAYBACK-{}", time_t));
            }

            auto me = plugin->ControllerMyself();

            // Send client info
            ClientInfo info;

            info.set_frequency(std::format("{:.3f}", me.GetPrimaryFrequency()));
            info.set_range(me.GetRange());
            info.mutable_session()->CopyFrom(session);

            auto runways = plugin->GetActiveRunways("EKCH");

            AirportInfo airport_info;
            for (auto [name, isDeparture]: runways) {
                const auto r = airport_info.add_runways();
                r->set_runway(name);
                r->set_departure(isDeparture);
            }

            info.mutable_airport_info()->CopyFrom(airport_info);

            ClientStreamMessage message;
            message.set_clientid(std::string(me.GetCallsign()));
            message.mutable_client_info()->CopyFrom(info);

            reader->AddMessage(message);
        }
    }

    void NetworkService::RadarTargetPositionEvent(EuroScopePlugIn::CRadarTarget radarTarget) {
        if (!ShouldSend()) return;

        const auto cPosition = radarTarget.GetPosition();

        LatLng latLng;
        latLng.set_latitude(cPosition.GetPosition().m_Latitude);
        latLng.set_longitude(cPosition.GetPosition().m_Longitude);

        Position p;
        p.set_altitude(cPosition.GetFlightLevel());
        p.mutable_position()->CopyFrom(latLng);
        PositionUpdate positionUpdate;
        positionUpdate.mutable_position()->CopyFrom(p);

        StripData data;
        data.set_callsign(std::string(radarTarget.GetCallsign()));
        data.mutable_position()->CopyFrom(positionUpdate);

        ClientStreamMessage message;
        message.set_clientid(position);
        message.mutable_strip_data()->CopyFrom(data);

        reader->AddMessage(message);
    }

    NetworkService::NetworkService(const std::shared_ptr<FlightStripsPlugin> &plugin, const std::shared_ptr<grpc::Channel> &channel) :
        plugin(plugin),
        client(channel) {
    }


    bool NetworkService::Online(int connection) {
        return connection == EuroScopePlugIn::CONNECTION_TYPE_DIRECT ||
               connection == EuroScopePlugIn::CONNECTION_TYPE_SWEATBOX ||
               connection == EuroScopePlugIn::CONNECTION_TYPE_PLAYBACK;
    }

    bool NetworkService::ShouldSend() const {
        return isMaster && Online(connectionStatus) && reader;
    }

    void NetworkService::OnNetworkMessage(const ServerStreamMessage& message) {
        if (message.has_session_info()) {
            const auto& info = message.session_info();
            isMaster = info.ismaster();
            if (isMaster) {
                plugin->Information("Master");
            }

            // TODO ensure that we do not send outdated information compared to what is on the server.
            // The master client need to sync all the extra state with the server before it can send.
            // Such as squawks and so on.
            // Sending all flight plans.
            for (auto fp = this->plugin->FlightPlanSelectFirst(); fp.IsValid(); fp = this->plugin->FlightPlanSelectNext(fp)) {
                if (!this->plugin->IsRelevant(fp)) {
                    continue;
                }

                auto full = ConvertToFullData(fp);

                StripData data;
                data.set_callsign(fp.GetCallsign());
                data.mutable_fulldata()->CopyFrom(full);

                ClientStreamMessage clientMessage;
                clientMessage.set_clientid(position);
                clientMessage.mutable_strip_data()->CopyFrom(data);

                reader->AddMessage(clientMessage);
            }
        }
    }

    Capabilities NetworkService::GetCapabilities(const EuroScopePlugIn::CFlightPlan& flightPlan) {
        switch (toupper(flightPlan.GetFlightPlanData().GetCapibilities())) {
            case 'T':
                return T;
            case 'X':
                return X;
            case 'U':
                return U;
            case 'D':
                return D;
            case 'B':
                return B;
            case 'A':
                return A;
            case 'M':
                return M;
            case 'N':
                return N;
            case 'P':
                return P;
            case 'Y':
                return Y;
            case 'C':
                return C;
            case 'I':
                return I;
            case 'E':
                return E;
            case 'F':
                return F;
            case 'G':
                return G;
            case 'R':
                return R;
            case 'W':
                return W;
            case 'Q':
                return Q;
            case '?':
            default:
                // Handle unexpected capability code
                return CAPIBILITIES_UNKNOWN;
        }
    }

    GroundState NetworkService::GetGroundState(const EuroScopePlugIn::CFlightPlan &flightPlan) {
        auto state = flightPlan.GetGroundState();
        if (strcmp(state, "ST-UP") == 0) return START_UP;
        if (strcmp(state, "PUSH") == 0) return PUSH;
        if (strcmp(state, "TAXI") == 0) return TAXI;
        if (strcmp(state, "DEPA") == 0) return DEPART;
        return GROUND_STATE_NONE;
    }

    WeightCategory NetworkService::GetAircraftWtc(const EuroScopePlugIn::CFlightPlan &flightPlan) {
        switch (flightPlan.GetFlightPlanData().GetAircraftWtc()) {
            case 'L':
                return LIGHT;
            case 'M':
                return MEDIUM;
            case 'H':
                return HEAVY;
            case 'J':
                return SUPER_HEAVY;
            case '?':
            default:
                return WEIGHT_CATEGORY_UNKNOWN;
        }
    }

    StripFullData NetworkService::ConvertToFullData(const EuroScopePlugIn::CFlightPlan &flightPlan) {
        StripFullData full;
        full.set_route(std::string(flightPlan.GetFlightPlanData().GetRoute()));
        full.set_origin(std::string(flightPlan.GetFlightPlanData().GetOrigin()));
        full.set_destination(std::string(flightPlan.GetFlightPlanData().GetDestination()));
        full.set_remarks(std::string(flightPlan.GetFlightPlanData().GetRemarks()));
        full.set_squawk(std::string(flightPlan.GetControllerAssignedData().GetSquawk()));
        full.set_sid(std::string(flightPlan.GetFlightPlanData().GetSidName())); // TODO don't set if not departure
        full.set_cleared_alt(flightPlan.GetControllerAssignedData().GetClearedAltitude());
        if (flightPlan.GetControllerAssignedData().GetAssignedHeading() != 0)
            full.set_heading(flightPlan.GetControllerAssignedData().GetAssignedHeading());
        full.set_aircraft_type(std::string(flightPlan.GetFlightPlanData().GetAircraftInfo()));
        full.set_runway(std::string(flightPlan.GetFlightPlanData().GetDepartureRwy()));
        full.set_cleared(flightPlan.GetClearenceFlag());
        full.set_final_altitude(flightPlan.GetControllerAssignedData().GetFinalAltitude());
        full.set_alternate(std::string(flightPlan.GetFlightPlanData().GetAlternate()));
        full.set_estimated_departure_time(std::string(flightPlan.GetFlightPlanData().GetEstimatedDepartureTime()));
        full.set_capabilities(GetCapabilities(flightPlan));
        full.set_communication_type(GetCommunicationType(flightPlan));
        full.set_ground_state(GetGroundState(flightPlan));
        full.set_aircraft_category(GetAircraftWtc(flightPlan));

        Position p;
        LatLng latLng;
        latLng.set_longitude(flightPlan.GetCorrelatedRadarTarget().GetPosition().GetPosition().m_Longitude);
        latLng.set_latitude(flightPlan.GetCorrelatedRadarTarget().GetPosition().GetPosition().m_Latitude);
        p.set_altitude(flightPlan.GetCorrelatedRadarTarget().GetPosition().GetPressureAltitude());
        p.mutable_position()->CopyFrom(latLng);
        full.mutable_position()->CopyFrom(p);

        return full;
    }

    NetworkService::~NetworkService() {
        if (reader) {
            reader->TryCancel();
            // Block until gRPC is done.
            reader->WaitForOnDone();
        }
    }
}

