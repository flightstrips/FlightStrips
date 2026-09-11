#pragma once

#include <string>

#include "Events.h"

namespace flightstrips::euroscope::v1 {
    class Envelope;
    class SessionInfoEvent;
    class RunwayMismatchAlertEvent;
    class CdmUpdateEvent;
    class CdmUpdateBatchEvent;
    class AssignedSquawkEvent;
    class RequestedAltitudeEvent;
    class ClearedAltitudeEvent;
    class CommunicationTypeEvent;
    class GroundStateEvent;
    class ClearedFlagEvent;
    class HeadingEvent;
    class StandEvent;
    class EobtEvent;
    class GenerateSquawkEvent;
    class RouteEvent;
    class RemarksEvent;
    class AircraftInfoEvent;
    class AircraftInfoRemarksEvent;
    class SidEvent;
    class AircraftRunwayEvent;
    class CoordinationHandoverEvent;
    class AssumeOnlyEvent;
    class AssumeAndDropEvent;
    class DropTrackingEvent;
    class BackendSyncEvent;
    class CreateFPLEvent;
    class PdcStateChangeEvent;
    class SendPrivateMessageEvent;
}

namespace FlightStrips::websocket::protobuf {
    namespace wire = ::flightstrips::euroscope::v1;

    std::string Serialize(const Event& event);
    bool ParseEnvelope(const std::string& bytes, wire::Envelope& envelope);
    EventType GetEventType(const wire::Envelope& envelope);

    void Decode(const wire::SessionInfoEvent& source, SessionInfoEvent& target);
    void Decode(const wire::RunwayMismatchAlertEvent& source, RunwayMismatchAlertEvent& target);
    void Decode(const wire::CdmUpdateEvent& source, CdmUpdateEvent& target);
    void Decode(const wire::CdmUpdateBatchEvent& source, CdmUpdateBatchEvent& target);
    void Decode(const wire::AssignedSquawkEvent& source, AssignedSquawkEvent& target);
    void Decode(const wire::RequestedAltitudeEvent& source, RequestedAltitudeEvent& target);
    void Decode(const wire::ClearedAltitudeEvent& source, ClearedAltitudeEvent& target);
    void Decode(const wire::CommunicationTypeEvent& source, CommunicationTypeEvent& target);
    void Decode(const wire::GroundStateEvent& source, GroundStateEvent& target);
    void Decode(const wire::ClearedFlagEvent& source, ClearedFlagEvent& target);
    void Decode(const wire::HeadingEvent& source, HeadingEvent& target);
    void Decode(const wire::StandEvent& source, StandEvent& target);
    void Decode(const wire::EobtEvent& source, EobtEvent& target);
    void Decode(const wire::GenerateSquawkEvent& source, GenerateSquawkEvent& target);
    void Decode(const wire::RouteEvent& source, RouteEvent& target);
    void Decode(const wire::RemarksEvent& source, RemarksEvent& target);
    void Decode(const wire::AircraftInfoEvent& source, AircraftInfoEvent& target);
    void Decode(const wire::AircraftInfoRemarksEvent& source, AircraftInfoRemarksEvent& target);
    void Decode(const wire::SidEvent& source, SidEvent& target);
    void Decode(const wire::AircraftRunwayEvent& source, AircraftRunwayEvent& target);
    void Decode(const wire::CoordinationHandoverEvent& source, CoordinationHandoverEvent& target);
    void Decode(const wire::AssumeOnlyEvent& source, AssumeOnlyEvent& target);
    void Decode(const wire::AssumeAndDropEvent& source, AssumeAndDropEvent& target);
    void Decode(const wire::DropTrackingEvent& source, DropTrackingEvent& target);
    void Decode(const wire::BackendSyncEvent& source, BackendSyncEvent& target);
    void Decode(const wire::CreateFPLEvent& source, CreateFPLEvent& target);
    void Decode(const wire::PdcStateChangeEvent& source, PdcStateChangeEvent& target);
    void Decode(const wire::SendPrivateMessageEvent& source, SendPrivateMessageEvent& target);
}
