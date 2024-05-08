using FlightStrips;
using Vatsim.Scandinavia.FlightStrips.Abstractions;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Enums;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips;
using Vatsim.Scandinavia.FlightStrips.Abstractions.Strips.Events;
using CommunicationType = FlightStrips.CommunicationType;

namespace Vatsim.Scandinavia.FlightStrips.Host.Mappers;

public interface IGRpcMapper
{
    FullStripEvent MapFull(StripData stripData, SessionId session);
    PositionEvent MapPosition(StripData stripData, SessionId session);
    AircraftCapabilities Map(Capabilities capabilities);
    StripState Map(GroundState state);
    Vatsim.Scandinavia.FlightStrips.Abstractions.Strips.CommunicationType Map(CommunicationType communicationType);
    StripId MapStripId(StripData stripData, SessionId session);
}
